package generate

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	_ "embed"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	// actionsFolder is the folder where the actions are stored
	actionsFolder = "internal"

	//go:embed action.go.tmpl
	actionTemplate string
)

// Action generates a new action
func Action(name string) error {
	folder := filepath.Dir(name)
	if folder == "." {
		folder = ""
	}

	fileName := filepath.Base(name)
	actionName := cases.Title(language.English).String(filepath.Base(name))

	actionPackage := "internal"
	parts := strings.Split(folder, string(filepath.Separator))
	if len(parts) > 1 {
		actionPackage = parts[len(parts)-1]
	}

	if folder != "" {
		// Create the folder
		if err := os.MkdirAll(filepath.Join(actionsFolder, folder), 0755); err != nil {
			return fmt.Errorf("error creating folder: %w", err)
		}
	}

	// Create action.go
	file, err := os.Create(filepath.Join(actionsFolder, folder, fileName+".go"))
	if err != nil {
		return err
	}

	defer file.Close()
	template := template.Must(template.New("handler").Parse(actionTemplate))
	err = template.Execute(file, map[string]string{
		"Package":  actionPackage,
		"FileName": fileName,
		"Folder":   folder,

		"ActionName": actionName,
	})

	// Create action.html
	_, err = os.Create(filepath.Join(actionsFolder, folder, fileName+".html"))
	if err != nil {
		return err
	}

	fmt.Println("Action files created successfully✅")

	return nil
}