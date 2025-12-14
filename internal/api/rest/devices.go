package rest

import (
	"fmt"
	"net/http"
	"time"

	"github.com/KevinKickass/OpenMachineCore/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// GET /api/v1/devices
func (s *Server) listDevices(c *gin.Context) {
	devices := s.lm.DeviceManager().ListDevices()

	response := make([]gin.H, 0, len(devices))
	for _, device := range devices {
		response = append(response, gin.H{
			"id":        device.ID,
			"name":      device.Name,
			"profile":   device.Profile.DeviceProfile.Model,
			"connected": device.Client != nil,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"devices": response,
		"count":   len(response),
	})
}

// GET /api/v1/devices/:id
func (s *Server) getDevice(c *gin.Context) {
	idStr := c.Param("id")
	deviceID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device ID"})
		return
	}

	device, exists := s.lm.DeviceManager().GetDevice(deviceID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         device.ID,
		"name":       device.Name,
		"profile":    device.Profile.DeviceProfile,
		"registers":  device.Profile.Registers,
		"io_mapping": device.IOMapping,
	})
}

// POST /api/v1/devices
func (s *Server) createDevice(c *gin.Context) {
	var req struct {
		InstanceID  string                  `json:"instance_id" binding:"required"`
		Composition types.CompositionConfig `json:"composition" binding:"required"`
		IOMapping   map[string]string       `json:"io_mapping" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	comp := types.DeviceComposition{
		InstanceID:  req.InstanceID,
		Composition: req.Composition,
		IOMapping:   req.IOMapping,
	}

	// Save to database first (upsert)
	deviceID, err := s.lm.Storage().SaveOrUpdateDeviceComposition(c.Request.Context(), comp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to save device: %v", err)})
		return
	}

	// Load device from composition
	device, err := s.lm.DeviceManager().LoadDeviceFromComposition(comp, 2*time.Second)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Start poller
	pollInterval := s.lm.Config().Modbus.DefaultPollInterval
	if err := s.lm.DeviceManager().StartPoller(device.ID, pollInterval); err != nil {
		s.logger.Warn("Failed to start poller", zap.Error(err))
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":         deviceID,
		"runtime_id": device.ID,
		"name":       device.Name,
		"message":    "Device created and persisted successfully",
	})
}

// DELETE /api/v1/devices/:id
func (s *Server) deleteDevice(c *gin.Context) {
	instanceID := c.Param("id")

	// Get device first
	device, exists := s.lm.DeviceManager().GetDeviceByName(instanceID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	// Disconnect device
	if err := device.Disconnect(); err != nil {
		s.logger.Warn("Failed to disconnect device", zap.Error(err))
	}

	// Delete from database
	if err := s.lm.Storage().DeleteDevice(c.Request.Context(), instanceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to delete from database: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Device deleted successfully",
	})
}

// POST /api/v1/devices/:id/read
func (s *Server) readRegister(c *gin.Context) {
	idStr := c.Param("id")
	deviceID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device ID"})
		return
	}

	var req struct {
		Register string `json:"register" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	device, exists := s.lm.DeviceManager().GetDevice(deviceID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	value, err := device.ReadLogical(c.Request.Context(), req.Register)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"register":  req.Register,
		"value":     value,
		"timestamp": time.Now().Unix(),
	})
}

// POST /api/v1/devices/:id/write
func (s *Server) writeRegister(c *gin.Context) {
	idStr := c.Param("id")
	deviceID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device ID"})
		return
	}

	var req struct {
		Register string      `json:"register" binding:"required"`
		Value    interface{} `json:"value" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	device, exists := s.lm.DeviceManager().GetDevice(deviceID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	if err := device.WriteLogical(c.Request.Context(), req.Register, req.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Register written successfully",
		"register": req.Register,
		"value":    req.Value,
	})
}
