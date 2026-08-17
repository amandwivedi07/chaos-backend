// Package request holds transport-agnostic request helpers shared by handlers.
package request

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// Pagination captures ?page=&limit=&sort=&order=&search= uniformly.
type Pagination struct {
	Page   int
	Limit  int
	Sort   string // whitelisted by the repository
	Order  string // asc | desc
	Search string
}

const (
	defaultPage  = 1
	defaultLimit = 20
	maxLimit     = 100
)

// ParsePagination reads pagination params with safe bounds.
func ParsePagination(c *gin.Context) Pagination {
	p := Pagination{
		Page:   intQuery(c, "page", defaultPage),
		Limit:  intQuery(c, "limit", defaultLimit),
		Sort:   c.Query("sort"),
		Order:  strings.ToLower(c.DefaultQuery("order", "desc")),
		Search: strings.TrimSpace(c.Query("search")),
	}
	if p.Page < 1 {
		p.Page = defaultPage
	}
	if p.Limit < 1 || p.Limit > maxLimit {
		p.Limit = defaultLimit
	}
	if p.Order != "asc" {
		p.Order = "desc"
	}
	return p
}

func (p Pagination) Offset() int { return (p.Page - 1) * p.Limit }

func intQuery(c *gin.Context, key string, fallback int) int {
	n, err := strconv.Atoi(c.Query(key))
	if err != nil {
		return fallback
	}
	return n
}
