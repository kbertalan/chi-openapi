package openapi

import (
	"encoding/json"
	"log/slog"
	"os"

	"github.com/go-chi/chi/v5"
)

type GenerateParams struct {
	Router         chi.Router
	Config         Config
	FilePath       string
	RenameFunction ModelNameFunc
}

// GenerateOpenAPISpecFile generates the OpenAPI spec and writes it to the given file path.
func GenerateOpenAPISpecFile(p *GenerateParams) error {
	slog.Debug("[openapi] GenerateOpenAPISpecFile: generating OpenAPI spec", "filePath", p.FilePath)

	ensureTypeIndex()

	renameFunc := p.RenameFunction
	if renameFunc == nil {
		renameFunc = DefaultModelNameFunc
	}

	gen := NewGeneratorWithCache(typeIndex)
	gen.SetModelNameFunc(renameFunc)

	spec := gen.GenerateSpec(p.Router, p.Config)

	slog.Debug("[openapi] GenerateOpenAPISpecFile: writing OpenAPI spec to file", "version", spec.Info.Version)

	file, err := os.Create(p.FilePath)
	if err != nil {
		slog.Error("[openapi] GenerateOpenAPISpecFile: failed to create file", "err", err, "path", p.FilePath)
		return err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err = enc.Encode(spec); err != nil {
		slog.Error("[openapi] GenerateOpenAPISpecFile: failed to write file", "err", err)
		return err
	}

	slog.Debug("[openapi] GenerateOpenAPISpecFile: openapi.json written successfully")
	return nil
}
