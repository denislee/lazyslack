package screen

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/user/lazyslack/internal/ui/component"
)

type avatarResultMsg struct {
	userID string
	avatar string
}

func fetchAvatarCmd(userID, imageURL string, width int) tea.Cmd {
	return func() tea.Msg {
		rendered, err := component.FetchAndRenderAvatar(imageURL, width)
		if err != nil {
			slog.Error("avatar fetch failed", "user", userID, "error", err)
			return nil
		}
		return avatarResultMsg{userID: userID, avatar: rendered}
	}
}
