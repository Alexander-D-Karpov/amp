package views

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

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
	pauseAllBtn     *widget.Button
	resumeAllBtn    *widget.Button
	downloads       []*types.DownloadProgress
	debug           bool
}

type downloadItemComponents struct {
	nameLabel     *widget.Label
	progressBar   *widget.ProgressBar
	statusLabel   *widget.Label
	speedLabel    *widget.Label
	retryBtn      *widget.Button
	cancelBtn     *widget.Button
	mainContainer *fyne.Container
}

func NewDownloadsView(downloadManager *download.Manager) *DownloadsView {
	dv := &DownloadsView{
		downloadManager: downloadManager,
		debug:           true,
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
			if err := dv.updateDownloadItem(id, obj); err != nil && dv.debug {
				log.Printf("[DOWNLOADS_VIEW] Failed to update download item %d: %v", id, err)
			}
		},
	)
}

func (dv *DownloadsView) createDownloadItem() fyne.CanvasObject {
	fileIcon := widget.NewIcon(theme.DocumentIcon())
	fileIcon.Resize(fyne.NewSize(32, 32))

	nameLabel := widget.NewLabel("Filename")
	nameLabel.TextStyle = fyne.TextStyle{Bold: true}
	nameLabel.Truncation = fyne.TextTruncateEllipsis

	progressBar := widget.NewProgressBar()
	progressBar.SetValue(0.0)

	statusLabel := widget.NewLabel("Pending")
	speedLabel := widget.NewLabel("")
	speedLabel.TextStyle = fyne.TextStyle{Italic: true}

	cancelBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), nil)
	cancelBtn.Resize(fyne.NewSize(24, 24))

	retryBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), nil)
	retryBtn.Resize(fyne.NewSize(24, 24))
	retryBtn.Hide()

	actionsContainer := container.NewHBox(retryBtn, cancelBtn)
	progressContainer := container.NewVBox(progressBar, container.NewHBox(statusLabel, speedLabel))
	infoContainer := container.NewVBox(nameLabel, progressContainer)

	mainContainer := container.NewBorder(nil, nil, fileIcon, actionsContainer, infoContainer)

	return mainContainer
}

func (dv *DownloadsView) updateDownloadItem(id widget.ListItemID, obj fyne.CanvasObject) error {
	if id >= len(dv.downloads) {
		return fmt.Errorf("id %d out of range (len=%d)", id, len(dv.downloads))
	}

	progress := dv.downloads[id]
	if progress == nil {
		return fmt.Errorf("nil progress at id %d", id)
	}

	mainContainer, ok := obj.(*fyne.Container)
	if !ok {
		return fmt.Errorf("expected *fyne.Container, got %T", obj)
	}

	// Extract widgets from the container structure
	components := dv.extractWidgetsFromContainer(mainContainer)
	if components == nil {
		return fmt.Errorf("could not extract widgets from container")
	}

	// Update filename
	filename := extractFilename(progress.URL)
	if progress.Filename != "" {
		filename = progress.Filename
	}
	components.nameLabel.SetText(filename)

	// Update progress bar
	components.progressBar.SetValue(progress.Progress / 100.0)

	// Update status
	components.statusLabel.SetText(progress.Status.String())

	// Update speed
	speedText := ""
	if progress.Speed > 0 && progress.Status == types.DownloadStatusDownloading {
		if progress.Speed > 1024*1024 {
			speedText = fmt.Sprintf("%.1f MB/s", progress.Speed/(1024*1024))
		} else if progress.Speed > 1024 {
			speedText = fmt.Sprintf("%.1f KB/s", progress.Speed/1024)
		} else {
			speedText = fmt.Sprintf("%.0f B/s", progress.Speed)
		}
	}
	components.speedLabel.SetText(speedText)

	// Update action buttons
	switch progress.Status {
	case types.DownloadStatusFailed:
		components.retryBtn.Show()
		components.cancelBtn.SetIcon(theme.DeleteIcon())
		components.retryBtn.OnTapped = func() {
			if dv.debug {
				log.Printf("[DOWNLOADS_VIEW] Retry requested for: %s", progress.Filename)
			}
		}
		components.cancelBtn.OnTapped = func() {
			dv.removeDownload(progress.URL)
		}

	case types.DownloadStatusDownloading:
		components.retryBtn.Hide()
		components.cancelBtn.SetIcon(theme.CancelIcon())
		components.cancelBtn.OnTapped = func() {
			dv.downloadManager.Cancel(progress.URL)
		}

	case types.DownloadStatusCompleted:
		components.retryBtn.Hide()
		components.cancelBtn.SetIcon(theme.DeleteIcon())
		components.cancelBtn.OnTapped = func() {
			dv.removeDownload(progress.URL)
		}

	default:
		components.retryBtn.Hide()
		components.cancelBtn.SetIcon(theme.CancelIcon())
		components.cancelBtn.OnTapped = func() {
			dv.downloadManager.Cancel(progress.URL)
		}
	}

	return nil
}

func (dv *DownloadsView) extractWidgetsFromContainer(container *fyne.Container) *downloadItemComponents {
	if len(container.Objects) < 3 {
		return nil
	}

	// Get the info container (center of border)
	var infoContainer *fyne.Container
	var actionsContainer *fyne.Container

	// Find the info and actions containers
	for _, obj := range container.Objects {
		if cont, ok := obj.(*fyne.Container); ok {
			if len(cont.Objects) >= 2 {
				// Check if this looks like the info container (has label + progress container)
				if _, isLabel := cont.Objects[0].(*widget.Label); isLabel {
					infoContainer = cont
				} else if len(cont.Objects) == 2 {
					// Check if this looks like actions container (has 2 buttons)
					if _, isBtn1 := cont.Objects[0].(*widget.Button); isBtn1 {
						if _, isBtn2 := cont.Objects[1].(*widget.Button); isBtn2 {
							actionsContainer = cont
						}
					}
				}
			}
		}
	}

	if infoContainer == nil || actionsContainer == nil {
		return nil
	}

	// Extract name label (first object in info container)
	nameLabel, ok := infoContainer.Objects[0].(*widget.Label)
	if !ok {
		return nil
	}

	// Extract progress container (second object in info container)
	progressContainer, ok := infoContainer.Objects[1].(*fyne.Container)
	if !ok || len(progressContainer.Objects) < 2 {
		return nil
	}

	// Extract progress bar (first object in progress container)
	progressBar, ok := progressContainer.Objects[0].(*widget.ProgressBar)
	if !ok {
		return nil
	}

	// Extract status container (second object in progress container)
	statusContainer, ok := progressContainer.Objects[1].(*fyne.Container)
	if !ok || len(statusContainer.Objects) < 2 {
		return nil
	}

	// Extract status and speed labels
	statusLabel, ok := statusContainer.Objects[0].(*widget.Label)
	if !ok {
		return nil
	}

	speedLabel, ok := statusContainer.Objects[1].(*widget.Label)
	if !ok {
		return nil
	}

	// Extract buttons from actions container
	retryBtn, ok := actionsContainer.Objects[0].(*widget.Button)
	if !ok {
		return nil
	}

	cancelBtn, ok := actionsContainer.Objects[1].(*widget.Button)
	if !ok {
		return nil
	}

	return &downloadItemComponents{
		nameLabel:     nameLabel,
		progressBar:   progressBar,
		statusLabel:   statusLabel,
		speedLabel:    speedLabel,
		retryBtn:      retryBtn,
		cancelBtn:     cancelBtn,
		mainContainer: container,
	}
}

func (dv *DownloadsView) setupLayout() {
	controlsContainer := container.NewHBox(
		dv.clearBtn,
		widget.NewSeparator(),
		dv.pauseAllBtn,
		dv.resumeAllBtn,
	)

	header := container.NewVBox(
		controlsContainer,
		dv.statusLabel,
		widget.NewSeparator(),
	)

	content := container.NewScroll(dv.downloadsList)
	dv.container = container.NewBorder(header, nil, nil, nil, content)
}

func (dv *DownloadsView) setupEventHandlers() {
	dv.downloadManager.OnProgress(func(progress *types.DownloadProgress) {
		fyne.Do(func() {
			dv.refreshDownloads()
			dv.updateStatus()
		})
	})

	dv.downloadManager.OnCompletion(func(task *download.Task) {
		fyne.Do(func() {
			dv.refreshDownloads()
			dv.updateStatus()
		})
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
		case types.DownloadStatusDownloading, types.DownloadStatusPending:
			active++
		case types.DownloadStatusCompleted:
			completed++
		case types.DownloadStatusFailed:
			failed++
		}
	}

	var statusText string
	if active > 0 {
		statusText = fmt.Sprintf("%d active, %d completed, %d failed", active, completed, failed)
	} else if len(dv.downloads) == 0 {
		statusText = "No downloads"
	} else {
		statusText = fmt.Sprintf("%d completed, %d failed", completed, failed)
	}

	dv.statusLabel.SetText(statusText)
}

func (dv *DownloadsView) clearCompleted() {
	dv.downloadManager.ClearCompleted()
	dv.refreshDownloads()
	dv.updateStatus()
}

func (dv *DownloadsView) pauseAll() {
	if dv.debug {
		log.Printf("[DOWNLOADS_VIEW] Pause all requested")
	}
}

func (dv *DownloadsView) resumeAll() {
	if dv.debug {
		log.Printf("[DOWNLOADS_VIEW] Resume all requested")
	}
}

func (dv *DownloadsView) removeDownload(url string) {
	if dv.debug {
		log.Printf("[DOWNLOADS_VIEW] Remove download requested for: %s", url)
	}
	dv.refreshDownloads()
}

func (dv *DownloadsView) Refresh() {
	dv.refreshDownloads()
	dv.updateStatus()
}

func (dv *DownloadsView) Container() *fyne.Container {
	return dv.container
}

func extractFilename(url string) string {
	if len(url) == 0 {
		return "Unknown"
	}

	filename := filepath.Base(url)

	if queryIndex := strings.Index(filename, "?"); queryIndex != -1 {
		filename = filename[:queryIndex]
	}

	if fragmentIndex := strings.Index(filename, "#"); fragmentIndex != -1 {
		filename = filename[:fragmentIndex]
	}

	if filename == "." || filename == "/" || filename == "" {
		return "download"
	}

	return filename
}
