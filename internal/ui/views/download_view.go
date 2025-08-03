package views

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/download"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type DownloadsView struct {
	downloadManager *download.Manager
	container       *fyne.Container
	downloadsList   *widget.List
	statusLabel     *widget.Label
	clearBtn        *widget.Button
	downloads       []*types.DownloadProgress
}

func NewDownloadsView(downloadManager *download.Manager) *DownloadsView {
	dv := &DownloadsView{
		downloadManager: downloadManager,
	}

	dv.setupWidgets()
	dv.setupLayout()
	dv.setupEventHandlers()
	return dv
}

func (dv *DownloadsView) setupWidgets() {
	dv.clearBtn = widget.NewButtonWithIcon("Clear Completed", theme.DeleteIcon(), dv.clearCompleted)
	dv.statusLabel = widget.NewLabel("No active downloads")

	dv.downloadsList = widget.NewList(
		func() int {
			return len(dv.downloads)
		},
		func() fyne.CanvasObject {
			return dv.createDownloadItem()
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			dv.updateDownloadItem(id, obj)
		},
	)
}

func (dv *DownloadsView) createDownloadItem() fyne.CanvasObject {
	fileIcon := widget.NewIcon(theme.DocumentIcon())
	nameLabel := widget.NewLabel("Filename")
	nameLabel.TextStyle = fyne.TextStyle{Bold: true}
	progressBar := widget.NewProgressBar()
	statusLabel := widget.NewLabel("Pending")

	info := container.NewVBox(nameLabel, progressBar, statusLabel)
	return container.NewBorder(nil, nil, fileIcon, nil, info)
}

func (dv *DownloadsView) updateDownloadItem(id widget.ListItemID, obj fyne.CanvasObject) {
	if id >= len(dv.downloads) {
		return
	}

	progress := dv.downloads[id]
	c := obj.(*fyne.Container)

	if len(c.Objects) >= 2 {
		info := c.Objects[1].(*fyne.Container)
		if len(info.Objects) >= 3 {
			nameLabel := info.Objects[0].(*widget.Label)
			progressBar := info.Objects[1].(*widget.ProgressBar)
			statusLabel := info.Objects[2].(*widget.Label)

			nameLabel.SetText(extractFilename(progress.URL))
			progressBar.SetValue(progress.Progress / 100.0)
			statusLabel.SetText(progress.Status.String())
		}
	}
}

func (dv *DownloadsView) setupLayout() {
	header := container.NewVBox(
		container.NewHBox(dv.clearBtn),
		dv.statusLabel,
		widget.NewSeparator(),
	)

	content := container.NewScroll(dv.downloadsList)
	dv.container = container.NewBorder(header, nil, nil, nil, content)
}

func (dv *DownloadsView) setupEventHandlers() {
	dv.downloadManager.OnProgress(func(progress *types.DownloadProgress) {
		dv.refreshDownloads()
		dv.updateStatus()
	})
	dv.refreshDownloads()
}

func (dv *DownloadsView) refreshDownloads() {
	dv.downloads = dv.downloadManager.GetAllDownloads()
	dv.downloadsList.Refresh()
}

func (dv *DownloadsView) updateStatus() {
	active := 0
	completed := 0
	failed := 0

	for _, download := range dv.downloads {
		switch download.Status {
		case types.DownloadStatusDownloading:
			active++
		case types.DownloadStatusCompleted:
			completed++
		case types.DownloadStatusFailed:
			failed++
		}
	}

	if active > 0 {
		dv.statusLabel.SetText(fmt.Sprintf("%d active, %d completed, %d failed", active, completed, failed))
	} else if len(dv.downloads) == 0 {
		dv.statusLabel.SetText("No downloads")
	} else {
		dv.statusLabel.SetText(fmt.Sprintf("%d completed, %d failed", completed, failed))
	}
}

func (dv *DownloadsView) clearCompleted() {
	dv.downloadManager.ClearCompleted()
	dv.refreshDownloads()
}

func (dv *DownloadsView) Refresh() {
	dv.refreshDownloads()
}

func (dv *DownloadsView) Container() *fyne.Container {
	return dv.container
}

func extractFilename(url string) string {
	if len(url) == 0 {
		return "Unknown"
	}

	lastSlash := -1
	for i := len(url) - 1; i >= 0; i-- {
		if url[i] == '/' {
			lastSlash = i
			break
		}
	}

	if lastSlash == -1 || lastSlash == len(url)-1 {
		return "download"
	}

	filename := url[lastSlash+1:]
	if queryIndex := findChar(filename, '?'); queryIndex != -1 {
		filename = filename[:queryIndex]
	}

	if filename == "" {
		return "download"
	}
	return filename
}

func findChar(str string, char rune) int {
	for i, c := range str {
		if c == char {
			return i
		}
	}
	return -1
}
