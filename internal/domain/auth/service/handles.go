package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/chaosapp/backend/internal/domain/user/repository"
)

// Email local-parts can contain characters that read badly as a handle; fold
// anything outside this set to a dot.
var handleUnsafe = regexp.MustCompile(`[^a-z0-9._-]`)

// handleFromEmail derives a stable @handle from the unique part of an address:
// "Aman.Dwivedi+space@gmail.com" → "aman.dwivedi". Returns "" when there is no
// email to work from (Apple's Hide My Email still gives us a relay address, so
// this is rare).
func handleFromEmail(email string) string {
	local, _, found := strings.Cut(strings.ToLower(strings.TrimSpace(email)), "@")
	if !found || local == "" {
		return ""
	}
	// Drop the +tag: it is the same mailbox, and it is noise in a handle.
	if plus := strings.IndexByte(local, '+'); plus > 0 {
		local = local[:plus]
	}
	local = strings.Trim(handleUnsafe.ReplaceAllString(local, "."), ".")
	if len(local) > 30 {
		local = local[:30]
	}
	return local
}

// uniqueHandle resolves collisions by suffixing a counter, so two people whose
// addresses share a local-part ("aman@gmail" / "aman@outlook") both get one.
func uniqueHandle(ctx context.Context, users repository.UserRepository, base string) string {
	if base == "" {
		return ""
	}
	candidate := base
	for i := 2; i < 100; i++ {
		taken, err := users.HandleTaken(ctx, candidate)
		if err != nil || !taken {
			return candidate // on lookup failure, let the unique index decide
		}
		candidate = fmt.Sprintf("%s%d", base, i)
	}
	return ""
}
