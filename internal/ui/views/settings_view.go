package views

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/config"
)

type SettingsView struct {
	cfg          *config.Config
	container    *container.Scroll
	parentWindow fyne.Window
	searchEntry  *widget.Entry

	apiURLEntry   *widget.Entry
	tokenEntry    *widget.Entry
	timeoutSlider *widget.Slider
	retriesSlider *widget.Slider

	cachePathEntry    *widget.Entry
	cacheSizeSlider   *widget.Slider
	autoDownloadCheck *widget.Check
	walModeCheck      *widget.Check

	sampleRateSelect *widget.Select
	bufferSizeSlider *widget.Slider
	volumeSlider     *widget.Slider
	crossfadeCheck   *widget.Check

	themeSelect       *widget.Select
	languageSelect    *widget.Select
	gridColumnsSlider *widget.Slider
	windowSizeEntry   *widget.Entry

	maxResultsSlider     *widget.Slider
	fuzzyThresholdSlider *widget.Slider
	typingSearchCheck    *widget.Check
	debounceSlider       *widget.Slider

	maxConcurrentSlider *widget.Slider
	chunkSizeSlider     *widget.Slider
	tempDirEntry        *widget.Entry

	saveBtn   *widget.Button
	resetBtn  *widget.Button
	exportBtn *widget.Button
	importBtn *widget.Button
	applyBtn  *widget.Button

	onSettingsChanged func()
	originalConfig    *config.Config
}

func NewSettingsView(cfg *config.Config) *SettingsView {
	sv := &SettingsView{
		cfg: cfg,
	}

	sv.originalConfig = sv.cloneConfig(cfg)

	sv.setupWidgets()
	sv.setupLayout()
	sv.loadSettings()

	return sv
}

func (sv *SettingsView) setupLayout() {
	apiCard := widget.NewCard("API Settings", "Configure connection to the music API", container.NewVBox(
		sv.createFormRow("Base URL:", sv.apiURLEntry),
		sv.createFormRow("API Token:", sv.tokenEntry),
		sv.createSliderRow("Timeout (seconds):", sv.timeoutSlider),
		sv.createSliderRow("Retry Attempts:", sv.retriesSlider),
	))

	storageCard := widget.NewCard("Storage Settings", "Configure local storage and caching", container.NewVBox(
		sv.createFormRow("Cache Directory:", sv.cachePathEntry),
		sv.createSliderRow("Max Cache Size (MB):", sv.cacheSizeSlider),
		sv.autoDownloadCheck,
		sv.walModeCheck,
	))

	audioCard := widget.NewCard("Audio Settings", "Configure audio playback options", container.NewVBox(
		sv.createFormRow("Sample Rate:", sv.sampleRateSelect),
		sv.createSliderRow("Buffer Size:", sv.bufferSizeSlider),
		sv.createSliderRow("Default Volume (%):", sv.volumeSlider),
		sv.crossfadeCheck,
	))

	uiCard := widget.NewCard("User Interface", "Customize the application appearance", container.NewVBox(
		sv.createFormRow("Theme:", sv.themeSelect),
		sv.createFormRow("Language:", sv.languageSelect),
		sv.createSliderRow("Grid Columns:", sv.gridColumnsSlider),
		sv.createFormRow("Window Size:", sv.windowSizeEntry),
	))

	searchCard := widget.NewCard("Search Settings", "Configure search behavior", container.NewVBox(
		sv.createSliderRow("Max Results:", sv.maxResultsSlider),
		sv.createSliderRow("Fuzzy Threshold:", sv.fuzzyThresholdSlider),
		sv.typingSearchCheck,
		sv.createSliderRow("Debounce (ms):", sv.debounceSlider),
	))

	downloadCard := widget.NewCard("Download Settings", "Configure download behavior", container.NewVBox(
		sv.createSliderRow("Max Concurrent Downloads:", sv.maxConcurrentSlider),
		sv.createSliderRow("Chunk Size (KB):", sv.chunkSizeSlider),
		sv.createFormRow("Temporary Directory:", sv.tempDirEntry),
	))

	actionsCard := widget.NewCard("Actions", "Save, reset, or manage configuration", container.NewVBox(
		container.NewHBox(sv.saveBtn, sv.applyBtn),
		container.NewHBox(sv.resetBtn),
		widget.NewSeparator(),
		container.NewHBox(sv.exportBtn, sv.importBtn),
	))

	content := container.NewVBox(
		widget.NewLabel("AMP Settings"),
		widget.NewSeparator(),
		apiCard,
		storageCard,
		audioCard,
		uiCard,
		searchCard,
		downloadCard,
		actionsCard,
	)

	sv.container = container.NewScroll(content)
}

func (sv *SettingsView) setupWidgets() {
	sv.searchEntry = widget.NewEntry()

	sv.apiURLEntry = widget.NewEntry()
	sv.apiURLEntry.SetPlaceHolder("https://api.example.com")

	sv.tokenEntry = widget.NewPasswordEntry()
	sv.tokenEntry.SetPlaceHolder("Your API token")

	sv.timeoutSlider = widget.NewSlider(5, 120)
	sv.timeoutSlider.Step = 5

	sv.retriesSlider = widget.NewSlider(1, 10)
	sv.retriesSlider.Step = 1

	sv.cachePathEntry = widget.NewEntry()
	sv.cachePathEntry.SetPlaceHolder("/path/to/cache")

	sv.cacheSizeSlider = widget.NewSlider(100, 10000)
	sv.cacheSizeSlider.Step = 100

	sv.autoDownloadCheck = widget.NewCheck("Auto-download played songs", nil)
	sv.walModeCheck = widget.NewCheck("Enable WAL mode (recommended)", nil)

	sv.sampleRateSelect = widget.NewSelect([]string{
		"22050", "44100", "48000", "96000",
	}, nil)

	sv.bufferSizeSlider = widget.NewSlider(1024, 16384)
	sv.bufferSizeSlider.Step = 1024

	sv.volumeSlider = widget.NewSlider(0, 100)
	sv.crossfadeCheck = widget.NewCheck("Enable crossfade", nil)

	sv.themeSelect = widget.NewSelect([]string{"light", "dark"}, nil)
	sv.languageSelect = widget.NewSelect([]string{
		"en", "es", "fr", "de", "ru", "zh", "ja",
	}, nil)

	sv.gridColumnsSlider = widget.NewSlider(2, 8)
	sv.gridColumnsSlider.Step = 1

	sv.windowSizeEntry = widget.NewEntry()
	sv.windowSizeEntry.SetPlaceHolder("1200x800")

	sv.maxResultsSlider = widget.NewSlider(10, 500)
	sv.maxResultsSlider.Step = 10

	sv.fuzzyThresholdSlider = widget.NewSlider(0.1, 1.0)
	sv.fuzzyThresholdSlider.Step = 0.1

	sv.typingSearchCheck = widget.NewCheck("Search while typing", nil)

	sv.debounceSlider = widget.NewSlider(100, 1000)
	sv.debounceSlider.Step = 50

	sv.maxConcurrentSlider = widget.NewSlider(1, 10)
	sv.maxConcurrentSlider.Step = 1

	sv.chunkSizeSlider = widget.NewSlider(64, 8192)
	sv.chunkSizeSlider.Step = 64

	sv.tempDirEntry = widget.NewEntry()
	sv.tempDirEntry.SetPlaceHolder("/path/to/temp")

	sv.saveBtn = widget.NewButtonWithIcon("Save Settings", theme.DocumentSaveIcon(), sv.saveSettings)
	sv.saveBtn.Importance = widget.HighImportance

	sv.resetBtn = widget.NewButtonWithIcon("Reset to Defaults", theme.ViewRefreshIcon(), sv.resetSettings)
	sv.applyBtn = widget.NewButtonWithIcon("Apply Changes", theme.ConfirmIcon(), sv.applySettings)
	sv.exportBtn = widget.NewButtonWithIcon("Export Config", theme.FolderOpenIcon(), sv.exportSettings)
	sv.importBtn = widget.NewButtonWithIcon("Import Config", theme.FolderIcon(), sv.importSettings)
}

func (sv *SettingsView) createFormRow(label string, comp fyne.CanvasObject) *fyne.Container {
	labelWidget := widget.NewLabel(label)
	labelWidget.Resize(fyne.NewSize(150, labelWidget.MinSize().Height))
	return container.NewBorder(nil, nil, labelWidget, nil, comp)
}

func (sv *SettingsView) createSliderRow(label string, slider *widget.Slider) *fyne.Container {
	labelWidget := widget.NewLabel(label)
	valueLabel := widget.NewLabel(fmt.Sprintf("%.0f", slider.Value))
	valueLabel.Resize(fyne.NewSize(50, valueLabel.MinSize().Height))

	originalOnChanged := slider.OnChanged
	slider.OnChanged = func(value float64) {
		if slider.Step >= 1 {
			valueLabel.SetText(fmt.Sprintf("%.0f", value))
		} else {
			valueLabel.SetText(fmt.Sprintf("%.1f", value))
		}
		if originalOnChanged != nil {
			originalOnChanged(value)
		}
	}

	return container.NewBorder(
		nil, nil,
		labelWidget,
		valueLabel,
		slider,
	)
}

func (sv *SettingsView) loadSettings() {
	sv.apiURLEntry.SetText(sv.cfg.API.BaseURL)
	sv.tokenEntry.SetText(sv.cfg.API.Token)
	sv.timeoutSlider.SetValue(float64(sv.cfg.API.Timeout))
	sv.retriesSlider.SetValue(float64(sv.cfg.API.Retries))

	sv.cachePathEntry.SetText(sv.cfg.Storage.CacheDir)
	sv.cacheSizeSlider.SetValue(float64(sv.cfg.Storage.MaxCacheSize / 1024 / 1024))
	sv.autoDownloadCheck.SetChecked(sv.cfg.Download.AutoDownload)
	sv.walModeCheck.SetChecked(sv.cfg.Storage.EnableWAL)

	sv.sampleRateSelect.SetSelected(fmt.Sprintf("%d", sv.cfg.Audio.SampleRate))
	sv.bufferSizeSlider.SetValue(float64(sv.cfg.Audio.BufferSize))
	sv.volumeSlider.SetValue(sv.cfg.Audio.DefaultVolume * 100)
	sv.crossfadeCheck.SetChecked(sv.cfg.Audio.Crossfade)

	sv.themeSelect.SetSelected(sv.cfg.UI.Theme)
	sv.languageSelect.SetSelected(sv.cfg.UI.Language)
	sv.gridColumnsSlider.SetValue(float64(sv.cfg.UI.GridColumns))
	sv.windowSizeEntry.SetText(fmt.Sprintf("%dx%d", sv.cfg.UI.WindowWidth, sv.cfg.UI.WindowHeight))

	sv.maxResultsSlider.SetValue(float64(sv.cfg.Search.MaxResults))
	sv.fuzzyThresholdSlider.SetValue(sv.cfg.Search.FuzzyThreshold)
	sv.typingSearchCheck.SetChecked(sv.cfg.Search.EnableTyping)
	sv.debounceSlider.SetValue(float64(sv.cfg.Search.DebounceMs))

	sv.maxConcurrentSlider.SetValue(float64(sv.cfg.Download.MaxConcurrent))
	sv.chunkSizeSlider.SetValue(float64(sv.cfg.Download.ChunkSize / 1024))
	sv.tempDirEntry.SetText(sv.cfg.Download.TempDir)
}

func (sv *SettingsView) applySettings() {
	sv.updateConfigFromUI()

	if sv.onSettingsChanged != nil {
		sv.onSettingsChanged()
	}

	sv.showInfo("Settings Applied", "Settings have been applied to the current session.")
}

func (sv *SettingsView) saveSettings() {
	sv.updateConfigFromUI()

	if err := sv.saveConfigToFile(); err != nil {
		sv.showError("Save Failed", err)
		return
	}

	if sv.onSettingsChanged != nil {
		sv.onSettingsChanged()
	}

	sv.showInfo("Settings Saved", "Your settings have been saved successfully!")
}

func (sv *SettingsView) updateConfigFromUI() {
	sv.cfg.API.BaseURL = sv.apiURLEntry.Text
	sv.cfg.API.Token = sv.tokenEntry.Text
	sv.cfg.API.Timeout = int(sv.timeoutSlider.Value)
	sv.cfg.API.Retries = int(sv.retriesSlider.Value)

	sv.cfg.Storage.CacheDir = sv.cachePathEntry.Text
	sv.cfg.Storage.MaxCacheSize = int64(sv.cacheSizeSlider.Value * 1024 * 1024)
	sv.cfg.Download.AutoDownload = sv.autoDownloadCheck.Checked
	sv.cfg.Storage.EnableWAL = sv.walModeCheck.Checked

	if rate, err := strconv.Atoi(sv.sampleRateSelect.Selected); err == nil {
		sv.cfg.Audio.SampleRate = rate
	}
	sv.cfg.Audio.BufferSize = int(sv.bufferSizeSlider.Value)
	sv.cfg.Audio.DefaultVolume = sv.volumeSlider.Value / 100.0
	sv.cfg.Audio.Crossfade = sv.crossfadeCheck.Checked

	sv.cfg.UI.Theme = sv.themeSelect.Selected
	sv.cfg.UI.Language = sv.languageSelect.Selected
	sv.cfg.UI.GridColumns = int(sv.gridColumnsSlider.Value)

	if windowSize := sv.windowSizeEntry.Text; windowSize != "" {
		var width, height int
		if n, err := fmt.Sscanf(windowSize, "%dx%d", &width, &height); n == 2 && err == nil {
			sv.cfg.UI.WindowWidth = width
			sv.cfg.UI.WindowHeight = height
		}
	}

	sv.cfg.Search.MaxResults = int(sv.maxResultsSlider.Value)
	sv.cfg.Search.FuzzyThreshold = sv.fuzzyThresholdSlider.Value
	sv.cfg.Search.EnableTyping = sv.typingSearchCheck.Checked
	sv.cfg.Search.DebounceMs = int(sv.debounceSlider.Value)

	sv.cfg.Download.MaxConcurrent = int(sv.maxConcurrentSlider.Value)
	sv.cfg.Download.ChunkSize = int(sv.chunkSizeSlider.Value * 1024)
	sv.cfg.Download.TempDir = sv.tempDirEntry.Text
}

func (sv *SettingsView) resetSettings() {
	dialog.ShowConfirm("Reset Settings", "Are you sure you want to reset all settings to their default values?", func(confirmed bool) {
		if confirmed {
			sv.cfg = sv.cloneConfig(sv.originalConfig)
			sv.loadSettings()
			sv.showInfo("Settings Reset", "All settings have been reset to their default values.")
		}
	}, sv.parentWindow)
}

func (sv *SettingsView) exportSettings() {
	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil || writer == nil {
			return
		}
		defer func() {
			if closeErr := writer.Close(); closeErr != nil {
				log.Printf("Failed to close file writer: %v", closeErr)
			}
		}()

		data, err := json.MarshalIndent(sv.cfg, "", "  ")
		if err != nil {
			sv.showError("Export Failed", err)
			return
		}

		if _, err := writer.Write(data); err != nil {
			sv.showError("Export Failed", err)
			return
		}

		sv.showInfo("Export Complete", "Settings have been exported successfully!")
	}, sv.parentWindow)
}

func (sv *SettingsView) importSettings() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer func() {
			if closeErr := reader.Close(); closeErr != nil {
				log.Printf("Failed to close file reader: %v", closeErr)
			}
		}()

		data, err := io.ReadAll(reader)
		if err != nil {
			sv.showError("Import Failed", err)
			return
		}

		var newCfg config.Config
		if err := json.Unmarshal(data, &newCfg); err != nil {
			sv.showError("Import Failed", fmt.Errorf("invalid configuration file: %v", err))
			return
		}

		sv.cfg = &newCfg
		sv.loadSettings()
		sv.showInfo("Import Complete", "Settings have been imported successfully!")
	}, sv.parentWindow)
}

func (sv *SettingsView) saveConfigToFile() error {
	return sv.cfg.Save()
}

func (sv *SettingsView) cloneConfig(cfg *config.Config) *config.Config {
	data, err := json.Marshal(cfg)
	if err != nil {
		log.Printf("Failed to marshal config for cloning: %v", err)
		return cfg
	}
	var clone config.Config
	if err := json.Unmarshal(data, &clone); err != nil {
		log.Printf("Failed to unmarshal config for cloning: %v", err)
		return cfg
	}
	return &clone
}

func (sv *SettingsView) showInfo(title, message string) {
	if sv.parentWindow != nil {
		dialog.ShowInformation(title, message, sv.parentWindow)
	}
}

func (sv *SettingsView) showError(title string, err error) {
	if sv.parentWindow != nil {
		dialog.ShowError(err, sv.parentWindow)
	}
}

func (sv *SettingsView) SetParentWindow(window fyne.Window) {
	sv.parentWindow = window
}

func (sv *SettingsView) OnSettingsChanged(callback func()) {
	sv.onSettingsChanged = callback
}

func (sv *SettingsView) Container() *fyne.Container {
	return container.NewStack(sv.container)
}

func (sv *SettingsView) Refresh() {
	sv.loadSettings()
}
