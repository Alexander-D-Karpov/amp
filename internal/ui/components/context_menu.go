package components

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type ContextMenu struct {
	song   *types.Song
	menu   *widget.PopUpMenu
	canvas fyne.Canvas

	onPlay        func(*types.Song)
	onLike        func(*types.Song)
	onDownload    func(*types.Song)
	onAddPlaylist func(*types.Song)
	debug         bool
}

func NewContextMenu(song *types.Song, debug bool) *ContextMenu {
	cm := &ContextMenu{
		song:  song,
		debug: debug,
	}
	return cm
}

func (cm *ContextMenu) createMenu(canvas fyne.Canvas) {
	if cm.song == nil || canvas == nil {
		if cm.debug {
			log.Printf("[CONTEXT_MENU] Cannot create menu - song: %v, canvas: %v", cm.song != nil, canvas != nil)
		}
		return
	}

	cm.canvas = canvas
	var menuItems []*fyne.MenuItem

	// Play song option
	playItem := fyne.NewMenuItem("Play", func() {
		if cm.debug {
			log.Printf("[CONTEXT_MENU] Play requested for: %s", cm.song.Name)
		}
		if cm.onPlay != nil {
			cm.onPlay(cm.song)
		}
		cm.Hide()
	})
	playItem.Icon = theme.MediaPlayIcon()
	menuItems = append(menuItems, playItem)

	// Separator
	menuItems = append(menuItems, fyne.NewMenuItemSeparator())

	// Like/Unlike option
	var likeItem *fyne.MenuItem
	if cm.song.Liked != nil && *cm.song.Liked {
		likeItem = fyne.NewMenuItem("Unlike", func() {
			if cm.debug {
				log.Printf("[CONTEXT_MENU] Unlike requested for: %s", cm.song.Name)
			}
			if cm.onLike != nil {
				cm.onLike(cm.song)
			}
			cm.Hide()
		})
		likeItem.Icon = theme.ConfirmIcon()
	} else {
		likeItem = fyne.NewMenuItem("Like", func() {
			if cm.debug {
				log.Printf("[CONTEXT_MENU] Like requested for: %s", cm.song.Name)
			}
			if cm.onLike != nil {
				cm.onLike(cm.song)
			}
			cm.Hide()
		})
		likeItem.Icon = theme.VisibilityOffIcon()
	}
	menuItems = append(menuItems, likeItem)

	// Download option
	var downloadItem *fyne.MenuItem
	if cm.song.Downloaded {
		downloadItem = fyne.NewMenuItem("Downloaded ✓", nil)
		downloadItem.Disabled = true
		downloadItem.Icon = theme.ConfirmIcon()
	} else {
		downloadItem = fyne.NewMenuItem("Download", func() {
			if cm.debug {
				log.Printf("[CONTEXT_MENU] Download requested for: %s", cm.song.Name)
			}
			if cm.onDownload != nil {
				cm.onDownload(cm.song)
			}
			cm.Hide()
		})
		downloadItem.Icon = theme.DownloadIcon()
	}
	menuItems = append(menuItems, downloadItem)

	// Add to playlist option
	playlistItem := fyne.NewMenuItem("Add to Playlist...", func() {
		if cm.debug {
			log.Printf("[CONTEXT_MENU] Add to playlist requested for: %s", cm.song.Name)
		}
		if cm.onAddPlaylist != nil {
			cm.onAddPlaylist(cm.song)
		}
		cm.Hide()
	})
	playlistItem.Icon = theme.ContentAddIcon()
	menuItems = append(menuItems, playlistItem)

	// Create the menu with proper canvas
	menu := fyne.NewMenu("", menuItems...)
	cm.menu = widget.NewPopUpMenu(menu, canvas)
}

func (cm *ContextMenu) ShowAt(canvas fyne.Canvas, pos fyne.Position) {
	if canvas == nil {
		if cm.debug {
			log.Printf("[CONTEXT_MENU] Cannot show menu - canvas is nil")
		}
		return
	}

	// Create menu with valid canvas
	cm.createMenu(canvas)

	if cm.menu != nil {
		if cm.debug {
			log.Printf("[CONTEXT_MENU] Showing context menu at position: %v", pos)
		}
		cm.menu.ShowAtPosition(pos)
	}
}

func (cm *ContextMenu) SetCallbacks(
	onPlay func(*types.Song),
	onLike func(*types.Song),
	onDownload func(*types.Song),
	onAddPlaylist func(*types.Song),
) {
	cm.onPlay = onPlay
	cm.onLike = onLike
	cm.onDownload = onDownload
	cm.onAddPlaylist = onAddPlaylist
}

func (cm *ContextMenu) Update(song *types.Song) {
	cm.song = song
	// Don't recreate menu here, let ShowAt handle it with proper canvas
}

func (cm *ContextMenu) Hide() {
	if cm.menu != nil {
		cm.menu.Hide()
	}
}
