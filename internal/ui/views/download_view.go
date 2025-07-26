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

	downloadsList *widget.List
	clearBtn      *widget.Button
	pauseAllBtn   *widget.Button
	resumeAllBtn  *widget.Button
	statusLabel   *widget.Label

	downloads []*types.DownloadProgress
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
	dv.pauseAllBtn = widget.NewButtonWithIcon("Pause All", theme.MediaPauseIcon(), dv.pauseAll)
	dv.resumeAllBtn = widget.NewButtonWithIcon("Resume All", theme.MediaPlayIcon(), dv.resumeAll)

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
	fileIcon.Resize(fyne.NewSize(32, 32))

	nameLabel := widget.NewLabel("Filename")
	nameLabel.TextStyle = fyne.TextStyle{Bold: true}

	urlLabel := widget.NewLabel("URL")
	urlLabel.TextStyle = fyne.TextStyle{Italic: true}

	progressBar := widget.NewProgressBar()
	progressBar.SetValue(0)

	statusLabel := widget.NewLabel("Pending")
	speedLabel := widget.NewLabel("0 KB/s")
	sizeLabel := widget.NewLabel("0 MB / 0 MB")

	cancelBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
	})
	cancelBtn.Importance = widget.LowImportance

	retryBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
	})
	retryBtn.Importance = widget.LowImportance
	retryBtn.Hide()

	fileInfo := container.NewVBox(
		nameLabel,
		urlLabel,
	)

	progressInfo := container.NewVBox(
		progressBar,
		container.NewHBox(
			statusLabel,
			widget.NewLabel("•"),
			speedLabel,
			widget.NewLabel("•"),
			sizeLabel,
		),
	)

	actions := container.NewHBox(cancelBtn, retryBtn)

	return container.NewBorder(
		nil, nil,
		fileIcon,
		actions,
		container.NewVBox(fileInfo, progressInfo),
	)
}

func (dv *DownloadsView) updateDownloadItem(id widget.ListItemID, obj fyne.CanvasObject) {
	if id >= len(dv.downloads) {
		return
	}

	progress := dv.downloads[id]
	c := obj.(*fyne.Container)

	var centerContent *fyne.Container
	var actions *fyne.Container

	for _, o := range c.Objects {
		if cont, ok := o.(*fyne.Container); ok {
			if len(cont.Objects) == 2 {
				centerContent = cont
			} else {
				actions = cont
			}
		}
	}

	if centerContent != nil && len(centerContent.Objects) >= 2 {
		fileInfoContainer := centerContent.Objects[0].(*fyne.Container)
		progressInfoContainer := centerContent.Objects[1].(*fyne.Container)

		if len(fileInfoContainer.Objects) >= 2 {
			nameLabel := fileInfoContainer.Objects[0].(*widget.Label)
			urlLabel := fileInfoContainer.Objects[1].(*widget.Label)

			if progress.Filename != "" {
				nameLabel.SetText(progress.Filename)
			} else {
				nameLabel.SetText(extractFilename(progress.URL))
			}
			urlLabel.SetText(progress.URL)
		}

		if len(progressInfoContainer.Objects) >= 2 {
			progressBar := progressInfoContainer.Objects[0].(*widget.ProgressBar)
			detailsContainer := progressInfoContainer.Objects[1].(*fyne.Container)

			progressBar.SetValue(progress.Progress / 100.0)

			if len(detailsContainer.Objects) >= 5 {
				statusLabel := detailsContainer.Objects[0].(*widget.Label)
				speedLabel := detailsContainer.Objects[2].(*widget.Label)
				sizeLabel := detailsContainer.Objects[4].(*widget.Label)

				statusLabel.SetText(progress.Status.String())

				if progress.Speed > 0 {
					speedLabel.SetText(formatSpeed(progress.Speed))
				} else {
					speedLabel.SetText("0 KB/s")
				}

				if progress.Total > 0 {
					sizeLabel.SetText(fmt.Sprintf("%s / %s",
						formatSize(progress.Downloaded),
						formatSize(progress.Total)))
				} else {
					sizeLabel.SetText(formatSize(progress.Downloaded))
				}
			}
		}
	}

	if actions != nil && len(actions.Objects) >= 2 {
		cancelBtn := actions.Objects[0].(*widget.Button)
		retryBtn := actions.Objects[1].(*widget.Button)

		switch progress.Status {
		case types.DownloadStatusPending, types.DownloadStatusDownloading:
			cancelBtn.Show()
			retryBtn.Hide()
			cancelBtn.OnTapped = func() {
				_ = dv.downloadManager.Cancel(progress.URL)
			}
		case types.DownloadStatusFailed, types.DownloadStatusCancelled:
			cancelBtn.Hide()
			retryBtn.Show()
			retryBtn.OnTapped = func() {
			}
		case types.DownloadStatusCompleted:
			cancelBtn.Hide()
			retryBtn.Hide()
		}
	}
}

func (dv *DownloadsView) setupLayout() {
	controlsContainer := container.NewHBox(
		dv.clearBtn,
		dv.pauseAllBtn,
		dv.resumeAllBtn,
	)

	header := container.NewVBox(
		controlsContainer,
		dv.statusLabel,
		widget.NewSeparator(),
	)

	content := container.NewScroll(dv.downloadsList)

	dv.container = container.NewBorder(
		header,
		nil,
		nil,
		nil,
		content,
	)
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

func (dv *DownloadsView) pauseAll() {
}

func (dv *DownloadsView) resumeAll() {
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

func formatSpeed(bytesPerSecond float64) string {
	if bytesPerSecond < 1024 {
		return fmt.Sprintf("%.0f B/s", bytesPerSecond)
	} else if bytesPerSecond < 1024*1024 {
		return fmt.Sprintf("%.1f KB/s", bytesPerSecond/1024)
	} else {
		return fmt.Sprintf("%.1f MB/s", bytesPerSecond/(1024*1024))
	}
}

func formatSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	} else if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	} else {
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
	}
}
