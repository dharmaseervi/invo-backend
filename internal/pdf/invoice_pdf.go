package pdf

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func GenerateInvoicePDF(
	templatePath string,
	data any,
	outputPath string,
) error {

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}

	tpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return err
	}

	var html bytes.Buffer
	if err := tpl.Execute(&html, data); err != nil {
		return err
	}

	tmpHTML := outputPath + ".html"
	if err := os.WriteFile(tmpHTML, html.Bytes(), 0644); err != nil {
		return err
	}
	defer os.Remove(tmpHTML)

	absHTML, err := filepath.Abs(tmpHTML)
	if err != nil {
		return err
	}

	htmlURL := "file://" + absHTML

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"--headless",
		"--disable-gpu",
		"--no-sandbox",
		"--print-to-pdf="+outputPath,
		htmlURL,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("chrome pdf failed: %w | %s", err, stderr.String())
	}

	return nil
}
