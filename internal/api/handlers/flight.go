package handlers

import (
	"net/http"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/service"
	"github.com/fjaeckel/pilotlog-api/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type FlightHandler struct {
	flightService *service.FlightService
	jwtManager    *jwt.Manager
}

func NewFlightHandler(flightService *service.FlightService, jwtManager *jwt.Manager) *FlightHandler {
	return &FlightHandler{
		flightService: flightService,
		jwtManager:    jwtManager,
	}
}

func (h *FlightHandler) CreateFlight(c *gin.Context) {
	userID, err := h.getUserIDFromToken(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	var flight models.Flight
	if err := c.ShouldBindJSON(&flight); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	flight.UserID = userID
	if err := h.flightService.CreateFlight(c.Request.Context(), &flight); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, flight)
}

func (h *FlightHandler) GetFlight(c *gin.Context) {
	userID, err := h.getUserIDFromToken(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	flightIDStr := c.Param("id")
	flightID, err := uuid.Parse(flightIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid flight ID"})
		return
	}

	flight, err := h.flightService.GetFlight(c.Request.Context(), flightID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "flight not found"})
		return
	}

	c.JSON(http.StatusOK, flight)
}

func (h *FlightHandler) ListFlights(c *gin.Context) {
	userID, err := h.getUserIDFromToken(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	flights, err := h.flightService.ListFlights(c.Request.Context(), userID, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, flights)
}

func (h *FlightHandler) UpdateFlight(c *gin.Context) {
	userID, err := h.getUserIDFromToken(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	flightIDStr := c.Param("id")
	flightID, err := uuid.Parse(flightIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid flight ID"})
		return
	}

	// Get existing flight
	flight, err := h.flightService.GetFlight(c.Request.Context(), flightID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "flight not found"})
		return
	}

	// Parse update fields
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Apply updates (simplified - in production would properly validate/convert all fields)
	if remarks, ok := updates["remarks"].(string); ok {
		flight.Remarks = &remarks
	}

	if err := h.flightService.UpdateFlight(c.Request.Context(), flight, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "flight updated successfully"})
}

func (h *FlightHandler) DeleteFlight(c *gin.Context) {
	userID, err := h.getUserIDFromToken(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	flightIDStr := c.Param("id")
	flightID, err := uuid.Parse(flightIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid flight ID"})
		return
	}

	if err := h.flightService.DeleteFlight(c.Request.Context(), flightID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "flight deleted successfully"})
}

func (h *FlightHandler) getUserIDFromToken(c *gin.Context) (uuid.UUID, error) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" || len(authHeader) < 8 {
		return uuid.Nil, jwt.ErrInvalidToken
	}

	tokenString := authHeader[7:]
	claims, err := h.jwtManager.ValidateAccessToken(tokenString)
	if err != nil {
		return uuid.Nil, err
	}

	return claims.UserID, nil
}
