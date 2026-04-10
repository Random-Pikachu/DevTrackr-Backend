package api

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/models"
	"github.com/Random-Pikachu/DevTrackr-Backend/internal/repository"
	"github.com/google/uuid"
)

type IntegrationHandler struct {
	integrationRepo *repository.IntegrationRepository
}

func NewIntegrationHandler(integrationRepo *repository.IntegrationRepository) *IntegrationHandler {
	return &IntegrationHandler{integrationRepo: integrationRepo}
}

func (h *IntegrationHandler) AddIntegration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID      string `json:"user_id"`
		Platform    string `json:"platform"`
		Handle      string `json:"handle"`
		AccessToken string `json:"access_token"`
		IsActive    *bool  `json:"is_active"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UserID == "" || req.Platform == "" || req.Handle == "" {
		writeError(w, http.StatusBadRequest, "user_id, platform and handle are required")
		return
	}

	userUUID, err := uuid.Parse(req.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user_id")
		return
	}

	accessToken := sql.NullString{String: req.AccessToken, Valid: req.AccessToken != ""}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	integration, err := h.integrationRepo.UpsertIntegration(r.Context(), models.Integration{
		UserID:      userUUID,
		Platform:    req.Platform,
		Handle:      req.Handle,
		AccessToken: accessToken,
		IsActive:    isActive,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("integration upserted user_id=%s integration_id=%s platform=%s handle=%s active=%t token=%t",
		req.UserID,
		integration.ID,
		req.Platform,
		req.Handle,
		integration.IsActive,
		accessToken.Valid,
	)

	writeJSON(w, http.StatusOK, integration)
}

func (h *IntegrationHandler) DeactivateIntegration(w http.ResponseWriter, r *http.Request) {
	integrationID := r.PathValue("id")
	if integrationID == "" {
		writeError(w, http.StatusBadRequest, "integration id is required")
		return
	}

	if err := h.integrationRepo.DeactivateIntegration(r.Context(), integrationID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("integration deactivated integration_id=%s", integrationID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "deactivated"})
}
