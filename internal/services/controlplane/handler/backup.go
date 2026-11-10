package handler

import (
	"errors"
	"fmt"
	"path/filepath"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// ListBackups reports the backup directory and its archives.
// @Summary      List backups
// @Tags         Backups
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} service.BackupList
// @Failure      401 {object} common.ErrorResponse
// @Router       /backups [get]
func (h *Handler) ListBackups(c fiber.Ctx) error {
	list, err := h.backups.List()
	if err != nil {
		return backupError(c, err)
	}
	return c.JSON(list)
}

// CreateBackup writes one archive per agent.
// @Summary      Create backups
// @Description  Writes a tar.gz archive per agent into the backup directory. An empty agent list backs up every initialized agent and adds the global registry.
// @Tags         Backups
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body service.BackupRequest false "What to include"
// @Success      200 {object} service.BackupCreated
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Router       /backups [post]
func (h *Handler) CreateBackup(c fiber.Ctx) error {
	var req service.BackupRequest
	if len(c.Body()) > 0 {
		if err := c.Bind().JSON(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
		}
	}

	created, err := h.backups.Create(req)
	if err != nil {
		return backupError(c, err)
	}
	return c.JSON(created)
}

// DownloadBackup streams one archive.
// @Summary      Download a backup
// @Tags         Backups
// @Produce      application/gzip
// @Security     BearerAuth
// @Param        name path string true "Archive name"
// @Success      200 {file} file "tar.gz archive"
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /backups/{name} [get]
func (h *Handler) DownloadBackup(c fiber.Ctx) error {
	path, err := h.backups.Archive(c.Params("name"))
	if err != nil {
		return backupError(c, err)
	}

	c.Set("Content-Type", "application/gzip")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(path)))
	c.Set("Cache-Control", "no-store")
	return c.SendFile(path)
}

// DeleteBackup removes one archive.
// @Summary      Delete a backup
// @Tags         Backups
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Archive name"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Router       /backups/{name} [delete]
func (h *Handler) DeleteBackup(c fiber.Ctx) error {
	if err := h.backups.Delete(c.Params("name")); err != nil {
		return backupError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// RestoreBackup applies an archive to the host.
// @Summary      Restore a backup
// @Description  Stops the agent if it is running, then rewrites its directory from the archive. force replaces an agent that already exists.
// @Tags         Backups
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Archive name"
// @Param        request body service.RestoreRequest false "Restore options"
// @Success      200 {object} service.RestoreResult
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /backups/{name}/restore [post]
func (h *Handler) RestoreBackup(c fiber.Ctx) error {
	var req service.RestoreRequest
	if len(c.Body()) > 0 {
		if err := c.Bind().JSON(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
		}
	}

	result, err := h.backups.Restore(c.Params("name"), req)
	if err != nil {
		return backupError(c, err)
	}
	return c.JSON(result)
}

func backupError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrBackupNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrBackupInvalid):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
