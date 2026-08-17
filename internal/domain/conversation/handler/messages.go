// Messages, Chaos turns, and the two things you can do to a Chaos turn:
// pick one of its options, or vote on the question it asked.
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/chaosapp/backend/internal/common/response"
	"github.com/chaosapp/backend/internal/domain/conversation/dto"
)

func (h *Handler) Messages(c *gin.Context) {
	userID, convID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	list, err := h.conversations.Messages(c.Request.Context(), convID, userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Messages fetched", list)
}

// Send godoc
// @Summary Say something to the group — Chaos answers when the turn earns it
// @Tags conversations
// @Success 201 {object} response.Body
// @Router /conversations/{id}/messages [post]
func (h *Handler) Send(c *gin.Context) {
	userID, convID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	req, ok := bind[dto.SendMessageRequest](c)
	if !ok {
		return
	}
	out, err := h.conversations.Send(c.Request.Context(), convID, userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, "Sent", out)
}

// Ask forces a Chaos turn regardless of the usual restraint.
func (h *Handler) Ask(c *gin.Context) {
	userID, convID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	reply, err := h.conversations.Ask(c.Request.Context(), convID, userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, "Chaos answered", reply)
}

func (h *Handler) MarkSeen(c *gin.Context) {
	userID, convID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	if err := h.conversations.MarkSeen(c.Request.Context(), convID, userID); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Seen", nil)
}

// Choose godoc
// @Summary Pick one of Chaos's options — says so in the thread
// @Tags conversations
// @Success 201 {object} response.Body
// @Router /conversations/{id}/choose [post]
func (h *Handler) Choose(c *gin.Context) {
	userID, convID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	req, ok := bind[dto.ChooseRequest](c)
	if !ok {
		return
	}
	message, err := h.conversations.Choose(c.Request.Context(), convID, userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, "Decided", message)
}

// Vote godoc
// @Summary Vote on a decision Chaos put to the group
// @Tags conversations
// @Success 200 {object} response.Body
// @Router /decisions/{id}/vote [post]
func (h *Handler) Vote(c *gin.Context) {
	userID, decisionID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	req, ok := bind[dto.VoteRequest](c)
	if !ok {
		return
	}
	decision, err := h.conversations.Vote(c.Request.Context(), decisionID, userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Voted", decision)
}
