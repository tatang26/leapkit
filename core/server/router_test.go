package server_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/leapkit/leapkit/core/server"
	"github.com/leapkit/leapkit/core/server/session"
)

func TestRouter(t *testing.T) {
	s := server.New()

	s.Group("/", func(r server.Router) {
		r.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("Hello, World!"))
		})

		r.Group("/api/", func(r server.Router) {
			r.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("This is the API!"))
			})

			r.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("API documentation!"))
			})

			r.Group("/v1/", func(r server.Router) {
				r.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
					w.Write([]byte("Welcome to the API v1!"))
				})

				r.Group("/users/", func(r server.Router) {
					r.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
						w.Write([]byte("Users list!"))
					})

					r.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
						w.Write([]byte("Hello users!"))
					})
				})
			})
		})
	})

	testCases := []struct {
		method string
		route  string
		body   string
		code   int
	}{
		{"GET", "/", "Hello, World!", http.StatusOK},
		{"GET", "/api/v1/users/hello", "Hello users!", http.StatusOK},
		{"GET", "/api/v1/users/", "Users list!", http.StatusOK},
		{"GET", "/api/v1/", "Welcome to the API v1!", http.StatusOK},
		{"GET", "/api/", "This is the API!", http.StatusOK},
		{"GET", "/api/docs", "API documentation!", http.StatusOK},
	}

	for _, tt := range testCases {
		t.Run(tt.route, func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, tt.route, nil)
			res := httptest.NewRecorder()
			s.Handler().ServeHTTP(res, req)

			if res.Code != tt.code {
				t.Errorf("Expected status code %d, got %d", tt.code, res.Code)
			}

			if res.Body.String() != tt.body {
				t.Errorf("Expected body %s, got %s", tt.body, res.Body.String())
			}
		})
	}

}

func TestMiddleware(t *testing.T) {
	t.Run("ResetMiddleware test", func(t *testing.T) {
		s := server.New()
		s.Use(server.InCtxMiddleware("customValue", "Hello, World!"))

		s.Group("/", func(r server.Router) {
			r.HandleFunc("GET /mw/{$}", func(w http.ResponseWriter, r *http.Request) {
				v := r.Context().Value("customValue").(string)
				w.Write([]byte(v))
			})

			r.Group("/without", func(r server.Router) {
				r.ResetMiddleware()

				r.HandleFunc("GET /mw/{$}", func(w http.ResponseWriter, r *http.Request) {
					v, ok := r.Context().Value("customValue").(string)
					if !ok {
						w.Write([]byte("customValue not found"))
						return
					}
					w.Write([]byte(v))
				})
			})

			r.Group("/other-with", func(r server.Router) {
				r.HandleFunc("GET /mw/{$}", func(w http.ResponseWriter, r *http.Request) {
					v := r.Context().Value("customValue").(string)
					w.Write([]byte(v + " (again)"))
				})
			})
		})

		testCases := []struct {
			description string
			pattern     string
			expected    string
		}{
			{"request to handler with middleware", "/mw/", "Hello, World!"},
			{"request to handler without middleware", "/without/mw/", "customValue not found"},
			{"request to other handler with middleware", "/other-with/mw/", "Hello, World! (again)"},
		}

		for _, tc := range testCases {
			t.Run(tc.description, func(t *testing.T) {
				req, _ := http.NewRequest(http.MethodGet, tc.pattern, nil)
				res := httptest.NewRecorder()
				s.Handler().ServeHTTP(res, req)

				if res.Body.String() != tc.expected {
					t.Errorf("Expected body %s, got %s", tc.expected, res.Body.String())
				}
			})
		}
	})

	t.Run("Middleware execution order", func(t *testing.T) {
		holder := []string{}

		mw := func(s string) func(http.Handler) http.Handler {
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					holder = append(holder, s)
					next.ServeHTTP(w, r)
				})
			}
		}

		s := server.New()

		s.Use(mw("one"))
		s.Use(mw("two"), mw("three"))

		s.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			holder = append(holder, "end")
		})

		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()
		s.Handler().ServeHTTP(res, req)

		expected := []string{"one", "two", "three", "end"}

		if slices.Compare(holder, expected) != 0 {
			t.Errorf("Expected order '%v', got '%v'", expected, holder)
		}
	})

	t.Run("WithSession Option", func(t *testing.T) {
		var req *http.Request
		ctx := context.Background()

		r := server.New(
			server.WithSession("secret_test", "test"),
		)

		r.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			sw := session.FromCtx(r.Context())
			sw.Values["greet"] = "Hello, World!"

			w.Write([]byte("ok"))

			// capturing the current context for a second call
			ctx = r.Context()
		})

		r.HandleFunc("GET /greet/{$}", func(w http.ResponseWriter, r *http.Request) {
			sw := session.FromCtx(r.Context())

			str := sw.Values["greet"].(string)

			w.Write([]byte(str))
		})

		resp := httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/", nil)
		r.Handler().ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Errorf("Expected response code %d, got %d", http.StatusOK, resp.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/greet/", nil)
		req = req.WithContext(ctx)
		resp.Body.Reset()

		r.Handler().ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Errorf("Expected response code %d, got %d", http.StatusOK, resp.Code)
		}

		if resp.Body.String() != "Hello, World!" {
			t.Errorf("Expected body %v, got %v", "Hello, World!", resp.Body.String())
		}
	})
}

func TestBaseMiddlewares(t *testing.T) {
	output := new(strings.Builder)
	log.SetOutput(output)

	t.Run("logger", func(t *testing.T) {
		t.Cleanup(output.Reset)

		s := server.New()
		s.Use(func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer output.Reset() // clear log output
				h.ServeHTTP(w, r)
			})
		})

		s.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
		})

		s.HandleFunc("GET /redirect/{$}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})

		s.HandleFunc("GET /error/{$}", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "error", http.StatusInternalServerError)
		})

		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		s.Handler().ServeHTTP(resp, req)

		if !strings.Contains(output.String(), "status=200") {
			t.Errorf("Expected log message %v, got %v", "status=200", output)
		}

		req = httptest.NewRequest(http.MethodGet, "/redirect/", nil)
		s.Handler().ServeHTTP(resp, req)

		if !strings.Contains(output.String(), "status=303") {
			t.Errorf("Expected log message %v, got %v", "status=303", output)
		}

		req = httptest.NewRequest(http.MethodGet, "/error/", nil)
		s.Handler().ServeHTTP(resp, req)

		if !strings.Contains(output.String(), "ERROR") {
			t.Errorf("Expected log message %v, got %v", "ERROR", output)
		}

		if !strings.Contains(output.String(), "status=500") {
			t.Errorf("Expected log message %v, got %v", "status=500", output)
		}
	})

	t.Run("recoverer error stack trace in development mode", func(t *testing.T) {
		current := os.Stderr

		r, testSrdErr, _ := os.Pipe()
		os.Stderr = testSrdErr

		t.Cleanup(func() {
			output.Reset()
			os.Stderr = current
		})

		t.Setenv("GO_ENV", "development")

		s := server.New()
		s.HandleFunc("GET /panic/{$}", func(w http.ResponseWriter, r *http.Request) {
			slice := [][]byte{}
			w.Write(slice[1])
		})

		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/panic/", nil)
		s.Handler().ServeHTTP(resp, req)

		if !strings.Contains(output.String(), "status=500") {
			t.Errorf("Expected log message %v, got %v", "status=500", output)
		}

		testSrdErr.Close()
		var buf bytes.Buffer
		io.Copy(&buf, r)

		if !strings.Contains(buf.String(), "runtime/debug.Stack()") {
			t.Errorf("Expected error message %v, got %v", "runtime/debug.Stack()", buf.String())
		}
	})

	t.Run("recoverer error stack trace in production mode", func(t *testing.T) {
		current := os.Stderr

		r, testSrdErr, _ := os.Pipe()
		os.Stderr = testSrdErr

		t.Cleanup(func() {
			output.Reset()
			os.Stderr = current
		})

		s := server.New()
		s.HandleFunc("GET /panic/{$}", func(w http.ResponseWriter, r *http.Request) {
			empty := [][]byte{}
			w.Write(empty[1])
		})

		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/panic/", nil)
		s.Handler().ServeHTTP(resp, req)

		if !strings.Contains(output.String(), "status=500") {
			t.Errorf("Expected log message %v, got %v", "status=500", output)
		}

		testSrdErr.Close()
		var buf bytes.Buffer
		io.Copy(&buf, r)

		if strings.Contains(buf.String(), "foo") {
			t.Errorf("Expected message to be empty, got %v", buf)
		}
	})
}

func TestCatchAll(t *testing.T) {
	t.Run("no catch-all defined", func(t *testing.T) {
		s := server.New()

		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/notregistered/one", nil)
		s.Handler().ServeHTTP(resp, req)

		if exp := "404 page not found"; !strings.Contains(resp.Body.String(), exp) {
			t.Errorf("Expected body %v, got %v", exp, resp.Body.String())
		}
	})

	t.Run("catch-all defined", func(t *testing.T) {
		s := server.New()
		s.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
		})

		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/notregistered/one", nil)
		s.Handler().ServeHTTP(resp, req)

		if exp := "ok"; !strings.Contains(resp.Body.String(), exp) {
			t.Errorf("Expected body %v, got %v", exp, resp.Body.String())
		}
	})

	t.Run("root with method defined", func(t *testing.T) {
		s := server.New()
		s.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
		})

		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/enters/the/get", nil)
		s.Handler().ServeHTTP(resp, req)

		if exp := "ok"; !strings.Contains(resp.Body.String(), exp) {
			t.Errorf("Expected GET body %v, got %v", exp, resp.Body.String())
		}

		resp = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/post/one", nil)
		s.Handler().ServeHTTP(resp, req)

		if exp := "not found"; !strings.Contains(resp.Body.String(), exp) {
			t.Errorf(": Expected body %v, got %v", exp, resp.Body.String())
		}
	})

}

func TestRegisterErrorHandler(t *testing.T) {
	t.Run("register 404 error page", func(t *testing.T) {
		s := server.New()
		// request using the default 404 page.
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/not/found/", nil)
		s.Handler().ServeHTTP(resp, req)

		if resp.Body.String() != "404 page not found" {
			t.Errorf("Expected `404 page not found`, got %v", resp.Body.String())
		}

		// Register custom 404 page
		s = server.New(
			server.WithErrorHandler(http.StatusNotFound, func(w http.ResponseWriter, r *http.Request, _ error) {
				w.Write([]byte("This is the custom not found page"))
			}),
		)

		resp = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/not/found/", nil)
		s.Handler().ServeHTTP(resp, req)

		if resp.Body.String() != "This is the custom not found page" {
			t.Errorf("Expected `This is the custom not found page`, got %v", resp.Body.String())
		}
	})

	t.Run("register 500 error page", func(t *testing.T) {
		boomHandler := func(w http.ResponseWriter, r *http.Request) {
			empty := [][]byte{}
			w.Write(empty[1])
		}

		s := server.New()
		s.HandleFunc("GET /boom/{$}", boomHandler)
		// request using the default 500 page.
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/boom/", nil)
		s.Handler().ServeHTTP(resp, req)

		if body := strings.TrimSpace(resp.Body.String()); body != "runtime error: index out of range [1] with length 0" {
			t.Errorf("Expected 'runtime error: index out of range [1] with length 0', got '%s'", body)
		}

		// Register custom 500 page
		s = server.New(
			server.WithErrorHandler(http.StatusInternalServerError, func(w http.ResponseWriter, r *http.Request, err error) {
				fmt.Fprintf(w, "This is the custom internal server error page :D : %v", err)
			}),
		)
		s.HandleFunc("GET /boom/{$}", boomHandler)

		resp = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/boom/", nil)
		s.Handler().ServeHTTP(resp, req)

		if body := strings.TrimSpace(resp.Body.String()); body != "This is the custom internal server error page :D : runtime error: index out of range [1] with length 0" {
			t.Errorf("Expected 'This is the custom internal server error page :D : runtime error: index out of range [1] with length 0', got %v", body)
		}
	})
}

// testHijackResponseWriter wraps the httptest.ResponseRecorder to implement Hijack interface.
type testHijackResponseWriter struct {
	*httptest.ResponseRecorder
}

func (t *testHijackResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}

func TestResponseWriter(t *testing.T) {
	s := server.New(
		server.WithSession("test_secret", "test"),
	)

	s.HandleFunc("GET /flush/{$}", func(w http.ResponseWriter, _ *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			w.Write([]byte("Flush not supported"))
			return
		}

		f.Flush()

		w.Write([]byte("Flush supported!"))
	})

	s.HandleFunc("GET /hijack/{$}", func(w http.ResponseWriter, _ *http.Request) {
		h, ok := w.(http.Hijacker)
		if !ok {
			w.Write([]byte("Hijack not supported"))
			return
		}

		_, _, err := h.Hijack()
		if err != nil {
			w.Write([]byte(err.Error()))
			return
		}

		w.Write([]byte("Hijack supported!"))
	})

	resp := &testHijackResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
	}
	req := httptest.NewRequest(http.MethodGet, "/flush/", nil)
	s.Handler().ServeHTTP(resp, req)

	if resp.Body.String() == "Flush not supported" {
		t.Errorf("Expected 'Flush supported!', got 'Flush not supported'")
		return
	}

	resp = &testHijackResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
	}

	req = httptest.NewRequest(http.MethodGet, "/hijack/", nil)
	s.Handler().ServeHTTP(resp, req)

	if resp.Body.String() == "Hijack not supported" {
		t.Errorf("Expected 'Hijack supported!', got 'Hijack not supported'")
		return
	}
}
