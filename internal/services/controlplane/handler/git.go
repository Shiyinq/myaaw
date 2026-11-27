package handler

import (
	"errors"
	"strconv"
	"strings"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// GetConversationGitStatus returns Git branch, ahead/behind, and changed files
// for the conversation's attached working directory.
// @Summary      Get project Git status
// @Tags         Git
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Success      200 {object} service.GitStatusResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/status [get]
func (h *Handler) GetConversationGitStatus(c fiber.Ctx) error {
	res, err := h.git.Status(c.Params("name"), c.Params("id"), h.requestUserID(c))
	if err != nil {
		return gitError(c, err)
	}
	return c.JSON(res)
}

// GetConversationGitLog returns paginated Git commits for the conversation's project.
// @Summary      List project Git commits
// @Tags         Git
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        limit query int false "Page size (default 20, max 100)"
// @Param        offset query int false "Commit offset (default 0)"
// @Success      200 {object} service.GitLogPageResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/log [get]
func (h *Handler) GetConversationGitLog(c fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	page, err := h.git.LogPage(c.Params("name"), c.Params("id"), h.requestUserID(c), limit, offset)
	if err != nil {
		return gitError(c, err)
	}
	return c.JSON(page)
}

// GetConversationGitDiffs returns aggregated diffs across all changed files
// (or all files within a commit) for the multi-diff viewer.
// @Summary      Get aggregated multi-file Git diff
// @Tags         Git
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        scope query string false "Diff scope: all (default), staged, or unstaged"
// @Param        commit query string false "Optional commit hash to diff all files in that commit"
// @Success      200 {object} service.GitMultiDiffResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/diffs [get]
func (h *Handler) GetConversationGitDiffs(c fiber.Ctx) error {
	scope := strings.TrimSpace(c.Query("scope"))
	commitHash := strings.TrimSpace(c.Query("commit"))
	res, err := h.git.DiffAll(c.Params("name"), c.Params("id"), h.requestUserID(c), scope, commitHash)
	if err != nil {
		return gitError(c, err)
	}
	return c.JSON(res)
}

// GetConversationGitDiff returns the unified diff for one changed file or one
// file inside a historical commit.
// @Summary      Get single-file Git diff
// @Tags         Git
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        path query string true "Repository-relative file path"
// @Param        staged query bool false "Read staged diff"
// @Param        untracked query bool false "Synthesize diff for an untracked file"
// @Param        commit query string false "Optional commit hash for historical commit diff"
// @Success      200 {object} service.GitDiffResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/diff [get]
func (h *Handler) GetConversationGitDiff(c fiber.Ctx) error {
	relPath := strings.TrimSpace(c.Query("path"))
	if relPath == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "path is required"})
	}
	commitHash := strings.TrimSpace(c.Query("commit"))
	if commitHash != "" {
		res, err := h.git.CommitDiff(c.Params("name"), c.Params("id"), h.requestUserID(c), commitHash, relPath)
		if err != nil {
			return gitError(c, err)
		}
		return c.JSON(res)
	}
	staged := queryBool(c.Query("staged"))
	untracked := queryBool(c.Query("untracked"))
	res, err := h.git.Diff(c.Params("name"), c.Params("id"), h.requestUserID(c), relPath, staged, untracked)
	if err != nil {
		return gitError(c, err)
	}
	return c.JSON(res)
}

// GetConversationGitCommitDetail returns commit metadata and changed files.
// @Summary      Get Git commit detail
// @Tags         Git
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        hash path string true "Commit hash"
// @Success      200 {object} service.GitCommitDetailResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/commits/{hash} [get]
func (h *Handler) GetConversationGitCommitDetail(c fiber.Ctx) error {
	res, err := h.git.CommitDetail(c.Params("name"), c.Params("id"), h.requestUserID(c), c.Params("hash"))
	if err != nil {
		return gitError(c, err)
	}
	return c.JSON(res)
}

type gitPathsRequest struct {
	Paths            []string `json:"paths"`
	All              bool     `json:"all"`
	IncludeUntracked bool     `json:"include_untracked"`
}

// StageConversationGit stages selected files or all changes.
// @Summary      Stage Git changes
// @Tags         Git
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        request body gitPathsRequest true "Paths to stage or all=true"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/stage [post]
func (h *Handler) StageConversationGit(c fiber.Ctx) error {
	var req gitPathsRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := h.git.Stage(c.Params("name"), c.Params("id"), h.requestUserID(c), req.Paths, req.All); err != nil {
		return gitError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// UnstageConversationGit unstages selected files or all staged changes.
// @Summary      Unstage Git changes
// @Tags         Git
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        request body gitPathsRequest true "Paths to unstage or all=true"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/unstage [post]
func (h *Handler) UnstageConversationGit(c fiber.Ctx) error {
	var req gitPathsRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := h.git.Unstage(c.Params("name"), c.Params("id"), h.requestUserID(c), req.Paths, req.All); err != nil {
		return gitError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// DiscardConversationGit discards unstaged edits or explicit untracked files.
// @Summary      Discard Git working-tree changes
// @Description  Restores tracked working-tree edits and optionally removes explicitly listed untracked non-ignored files (never deletes ignored files).
// @Tags         Git
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        request body gitPathsRequest true "Paths to discard or all=true"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/discard [post]
func (h *Handler) DiscardConversationGit(c fiber.Ctx) error {
	var req gitPathsRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := h.git.Discard(c.Params("name"), c.Params("id"), h.requestUserID(c), req.Paths, req.All, req.IncludeUntracked); err != nil {
		return gitError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

type gitCommitRequest struct {
	Message string `json:"message"`
	Amend   bool   `json:"amend"`
}

// CommitConversationGit creates or amends a commit.
// @Summary      Commit staged Git changes
// @Tags         Git
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        request body gitCommitRequest true "Commit message and optional amend flag"
// @Success      200 {object} map[string]service.GitCommitEntry
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/commit [post]
func (h *Handler) CommitConversationGit(c fiber.Ctx) error {
	var req gitCommitRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	commit, err := h.git.Commit(c.Params("name"), c.Params("id"), h.requestUserID(c), req.Message, req.Amend)
	if err != nil {
		return gitError(c, err)
	}
	return c.JSON(fiber.Map{"commit": commit})
}

// GetConversationGitBranches returns local and remote branch names.
// @Summary      List Git branches
// @Tags         Git
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Success      200 {object} service.GitBranchesResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/branches [get]
func (h *Handler) GetConversationGitBranches(c fiber.Ctx) error {
	res, err := h.git.Branches(c.Params("name"), c.Params("id"), h.requestUserID(c))
	if err != nil {
		return gitError(c, err)
	}
	return c.JSON(res)
}

type gitCheckoutRequest struct {
	Branch string `json:"branch"`
	Create bool   `json:"create"`
	From   string `json:"from"`
}

// CheckoutConversationGit switches to or creates a branch (optionally from a base ref).
// @Summary      Switch or create Git branch
// @Tags         Git
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        request body gitCheckoutRequest true "Target branch, optional create flag, and optional base ref (from)"
// @Success      200 {object} service.GitStatusResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/checkout [post]
func (h *Handler) CheckoutConversationGit(c fiber.Ctx) error {
	var req gitCheckoutRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	st, err := h.git.Checkout(c.Params("name"), c.Params("id"), h.requestUserID(c), req.Branch, req.Create, req.From)
	if err != nil {
		return gitError(c, err)
	}
	return c.JSON(st)
}

// FetchConversationGit runs non-interactive `git fetch --prune`.
// @Summary      Git fetch
// @Tags         Git
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Success      200 {object} service.GitActionResult
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/fetch [post]
func (h *Handler) FetchConversationGit(c fiber.Ctx) error {
	res, err := h.git.Fetch(c.Params("name"), c.Params("id"), h.requestUserID(c))
	if err != nil {
		return gitError(c, err)
	}
	return c.JSON(res)
}

// PullConversationGit runs non-interactive `git pull --ff-only`.
// @Summary      Git pull (fast-forward only)
// @Tags         Git
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Success      200 {object} service.GitActionResult
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/pull [post]
func (h *Handler) PullConversationGit(c fiber.Ctx) error {
	res, err := h.git.Pull(c.Params("name"), c.Params("id"), h.requestUserID(c))
	if err != nil {
		return gitError(c, err)
	}
	return c.JSON(res)
}

type gitPushRequest struct {
	SetUpstream bool `json:"set_upstream"`
}

// PushConversationGit pushes the active branch to its remote.
// @Summary      Git push
// @Tags         Git
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        request body gitPushRequest false "Optional set_upstream flag"
// @Success      200 {object} service.GitActionResult
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/push [post]
func (h *Handler) PushConversationGit(c fiber.Ctx) error {
	var req gitPushRequest
	_ = c.Bind().Body(&req)
	res, err := h.git.Push(c.Params("name"), c.Params("id"), h.requestUserID(c), req.SetUpstream)
	if err != nil {
		return gitError(c, err)
	}
	return c.JSON(res)
}

// GetConversationGitStashes lists repository stashes.
// @Summary      List Git stashes
// @Tags         Git
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Success      200 {object} map[string][]service.GitStashEntry
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/stashes [get]
func (h *Handler) GetConversationGitStashes(c fiber.Ctx) error {
	stashes, err := h.git.Stashes(c.Params("name"), c.Params("id"), h.requestUserID(c))
	if err != nil {
		return gitError(c, err)
	}
	return c.JSON(fiber.Map{"stashes": stashes})
}

type gitStashSaveRequest struct {
	Message          string `json:"message"`
	IncludeUntracked bool   `json:"include_untracked"`
}

// SaveConversationGitStash pushes current working-tree changes to a new stash.
// @Summary      Save Git stash
// @Tags         Git
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        request body gitStashSaveRequest false "Optional stash message and include_untracked flag"
// @Success      200 {object} service.GitActionResult
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/stashes [post]
func (h *Handler) SaveConversationGitStash(c fiber.Ctx) error {
	var req gitStashSaveRequest
	_ = c.Bind().Body(&req)
	res, err := h.git.StashSave(c.Params("name"), c.Params("id"), h.requestUserID(c), req.Message, req.IncludeUntracked)
	if err != nil {
		return gitError(c, err)
	}
	return c.JSON(res)
}

type gitStashRefRequest struct {
	Ref string `json:"ref"`
}

// ApplyConversationGitStash applies a stash without dropping it.
// @Summary      Apply Git stash
// @Tags         Git
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        request body gitStashRefRequest false "Stash ref (e.g. stash@{0})"
// @Success      200 {object} service.GitActionResult
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/stashes/apply [post]
func (h *Handler) ApplyConversationGitStash(c fiber.Ctx) error {
	var req gitStashRefRequest
	_ = c.Bind().Body(&req)
	res, err := h.git.StashApply(c.Params("name"), c.Params("id"), h.requestUserID(c), req.Ref)
	if err != nil {
		return gitError(c, err)
	}
	return c.JSON(res)
}

// PopConversationGitStash applies and removes a stash.
// @Summary      Pop Git stash
// @Tags         Git
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        request body gitStashRefRequest false "Stash ref (e.g. stash@{0})"
// @Success      200 {object} service.GitActionResult
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/stashes/pop [post]
func (h *Handler) PopConversationGitStash(c fiber.Ctx) error {
	var req gitStashRefRequest
	_ = c.Bind().Body(&req)
	res, err := h.git.StashPop(c.Params("name"), c.Params("id"), h.requestUserID(c), req.Ref)
	if err != nil {
		return gitError(c, err)
	}
	return c.JSON(res)
}

// DropConversationGitStash permanently deletes a stash entry.
// @Summary      Drop Git stash
// @Tags         Git
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        request body gitStashRefRequest true "Stash ref (e.g. stash@{0})"
// @Success      200 {object} service.GitActionResult
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/git/stashes/drop [post]
func (h *Handler) DropConversationGitStash(c fiber.Ctx) error {
	var req gitStashRefRequest
	_ = c.Bind().Body(&req)
	res, err := h.git.StashDrop(c.Params("name"), c.Params("id"), h.requestUserID(c), req.Ref)
	if err != nil {
		return gitError(c, err)
	}
	return c.JSON(res)
}

func gitError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrNoWorkingDir),
		errors.Is(err, service.ErrNotGitRepo),
		errors.Is(err, service.ErrInvalidPath),
		errors.Is(err, service.ErrInvalidGitArg):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrPathNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "path not found"})
	default:
		return conversationError(c, err)
	}
}
