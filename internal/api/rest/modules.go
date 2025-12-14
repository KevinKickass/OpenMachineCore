package rest

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type VendorIndex struct {
	Vendor      string                 `yaml:"vendor"`
	Description string                 `yaml:"description"`
	Website     string                 `yaml:"website"`
	Modules     map[string][]ModuleRef `yaml:"modules"`
}

type ModuleRef struct {
	ID          string `yaml:"id"`
	File        string `yaml:"file"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Tested      bool   `yaml:"tested"`
	Datasheet   string `yaml:"datasheet"`
}

// GET /api/v1/modules
func (s *Server) listModules(c *gin.Context) {
	searchPaths := s.lm.Config().Devices.SearchPaths

	s.logger.Info("Listing modules", zap.Strings("search_paths", searchPaths))

	vendors := make([]gin.H, 0)

	for _, searchPath := range searchPaths {
		// searchPath bereits "device-descriptors/vendors", nicht nochmal /vendors anhängen
		vendorsPath := searchPath

		s.logger.Debug("Checking vendors path", zap.String("path", vendorsPath))

		// Check if vendors directory exists
		if _, err := os.Stat(vendorsPath); os.IsNotExist(err) {
			s.logger.Warn("Vendors directory does not exist", zap.String("path", vendorsPath))
			continue
		}

		entries, err := os.ReadDir(vendorsPath)
		if err != nil {
			s.logger.Error("Failed to read vendors directory",
				zap.String("path", vendorsPath),
				zap.Error(err))
			continue
		}

		s.logger.Debug("Found vendor directories", zap.Int("count", len(entries)))

		for _, entry := range entries {
			if !entry.IsDir() {
				s.logger.Debug("Skipping non-directory", zap.String("name", entry.Name()))
				continue
			}

			vendorName := entry.Name()
			indexPath := filepath.Join(vendorsPath, vendorName, "index.yaml")

			s.logger.Debug("Checking vendor index",
				zap.String("vendor", vendorName),
				zap.String("index_path", indexPath))

			// Check if index.yaml exists
			if _, err := os.Stat(indexPath); os.IsNotExist(err) {
				s.logger.Warn("Vendor index not found",
					zap.String("vendor", vendorName),
					zap.String("path", indexPath))
				continue
			}

			// Read and parse index.yaml
			data, err := os.ReadFile(indexPath)
			if err != nil {
				s.logger.Error("Failed to read vendor index",
					zap.String("vendor", vendorName),
					zap.String("path", indexPath),
					zap.Error(err))
				continue
			}

			var index VendorIndex
			if err := yaml.Unmarshal(data, &index); err != nil {
				s.logger.Error("Failed to parse vendor index",
					zap.String("vendor", vendorName),
					zap.String("path", indexPath),
					zap.Error(err))
				continue
			}

			// Collect all modules from all categories
			modules := make([]ModuleRef, 0)
			for category, categoryModules := range index.Modules {
				s.logger.Debug("Found module category",
					zap.String("vendor", vendorName),
					zap.String("category", category),
					zap.Int("count", len(categoryModules)))
				modules = append(modules, categoryModules...)
			}

			s.logger.Info("Loaded vendor",
				zap.String("vendor", index.Vendor),
				zap.Int("module_count", len(modules)))

			vendors = append(vendors, gin.H{
				"vendor":       index.Vendor,
				"description":  index.Description,
				"website":      index.Website,
				"modules":      modules,
				"module_count": len(modules),
			})
		}
	}

	s.logger.Info("Total vendors loaded", zap.Int("count", len(vendors)))

	c.JSON(http.StatusOK, gin.H{
		"vendors": vendors,
		"count":   len(vendors),
	})
}

// GET /api/v1/modules/:vendor
func (s *Server) getVendorModules(c *gin.Context) {
	vendor := c.Param("vendor")

	s.logger.Info("Getting vendor modules", zap.String("vendor", vendor))

	searchPaths := s.lm.Config().Devices.SearchPaths

	for _, searchPath := range searchPaths {
		indexPath := filepath.Join(searchPath, vendor, "index.yaml")

		s.logger.Debug("Checking vendor index", zap.String("path", indexPath))

		if _, err := os.Stat(indexPath); os.IsNotExist(err) {
			s.logger.Debug("Index not found", zap.String("path", indexPath))
			continue
		}

		data, err := os.ReadFile(indexPath)
		if err != nil {
			s.logger.Error("Failed to read vendor index",
				zap.String("vendor", vendor),
				zap.Error(err))
			continue
		}

		var index VendorIndex
		if err := yaml.Unmarshal(data, &index); err != nil {
			s.logger.Error("Failed to parse vendor index",
				zap.String("vendor", vendor),
				zap.Error(err))
			continue
		}

		s.logger.Info("Vendor found",
			zap.String("vendor", index.Vendor),
			zap.Int("categories", len(index.Modules)))

		c.JSON(http.StatusOK, gin.H{
			"vendor":      index.Vendor,
			"description": index.Description,
			"website":     index.Website,
			"modules":     index.Modules,
		})
		return
	}

	s.logger.Warn("Vendor not found", zap.String("vendor", vendor))

	c.JSON(http.StatusNotFound, gin.H{
		"error":  "Vendor not found",
		"vendor": vendor,
	})
}

// GET /api/v1/modules/:vendor/:model
func (s *Server) getModule(c *gin.Context) {
	vendor := c.Param("vendor")
	model := c.Param("model")

	s.logger.Info("Getting module",
		zap.String("vendor", vendor),
		zap.String("model", model))

	searchPaths := s.lm.Config().Devices.SearchPaths

	for _, searchPath := range searchPaths {
		vendorPath := filepath.Join(searchPath, vendor)
		indexPath := filepath.Join(vendorPath, "index.yaml")

		s.logger.Debug("Checking vendor path",
			zap.String("path", vendorPath),
			zap.String("index", indexPath))

		// Read vendor index to find module file
		if _, err := os.Stat(indexPath); os.IsNotExist(err) {
			s.logger.Debug("Index not found", zap.String("path", indexPath))
			continue
		}

		data, err := os.ReadFile(indexPath)
		if err != nil {
			s.logger.Error("Failed to read index", zap.Error(err))
			continue
		}

		var index VendorIndex
		if err := yaml.Unmarshal(data, &index); err != nil {
			s.logger.Error("Failed to parse index", zap.Error(err))
			continue
		}

		// Find module in index (case-insensitive search)
		var moduleFile string
		modelLower := strings.ToLower(model)

		for category, categoryModules := range index.Modules {
			s.logger.Debug("Searching in category",
				zap.String("category", category),
				zap.Int("modules", len(categoryModules)))

			for _, mod := range categoryModules {
				s.logger.Debug("Checking module",
					zap.String("id", mod.ID),
					zap.String("name", mod.Name),
					zap.String("file", mod.File))

				if strings.ToLower(mod.Name) == modelLower ||
					strings.ToLower(mod.ID) == strings.ToLower(vendor+"-"+model) ||
					strings.ToLower(mod.ID) == modelLower {
					moduleFile = mod.File
					s.logger.Info("Found module match", zap.String("file", moduleFile))
					break
				}
			}
			if moduleFile != "" {
				break
			}
		}

		if moduleFile == "" {
			s.logger.Warn("Module not found in index",
				zap.String("vendor", vendor),
				zap.String("model", model))
			continue
		}

		// Read module JSON file
		modulePath := filepath.Join(vendorPath, moduleFile)

		s.logger.Info("Reading module file", zap.String("path", modulePath))

		if _, err := os.Stat(modulePath); os.IsNotExist(err) {
			s.logger.Error("Module file not found", zap.String("path", modulePath))
			continue
		}

		moduleData, err := os.ReadFile(modulePath)
		if err != nil {
			s.logger.Error("Failed to read module file",
				zap.String("path", modulePath),
				zap.Error(err))
			continue
		}

		// Parse JSON to validate it
		var moduleJSON map[string]interface{}
		if err := json.Unmarshal(moduleData, &moduleJSON); err != nil {
			s.logger.Error("Failed to parse module JSON",
				zap.String("path", modulePath),
				zap.Error(err))
			continue
		}

		s.logger.Info("Module loaded successfully",
			zap.String("vendor", vendor),
			zap.String("model", model))

		c.JSON(http.StatusOK, moduleJSON)
		return
	}

	s.logger.Warn("Module not found anywhere",
		zap.String("vendor", vendor),
		zap.String("model", model))

	c.JSON(http.StatusNotFound, gin.H{
		"error":  "Module not found",
		"vendor": vendor,
		"model":  model,
	})
}
