package components

import (
	"context"
	"log"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Alexander-D-Karpov/amp/internal/api"
)

type AuthDialog struct {
	api             *api.Client
	onAuthenticated func(string)
	parentWindow    fyne.Window

	dialog      dialog.Dialog
	tokenEntry  *widget.Entry
	loginBtn    *widget.Button
	cancelBtn   *widget.Button
	statusLabel *widget.Label
	progressBar *widget.ProgressBar
	helpLink    *widget.Hyperlink
}

func NewAuthDialog(api *api.Client) *AuthDialog {
	return &AuthDialog{
		api: api,
	}
}

func (ad *AuthDialog) Show(parent fyne.Window) {
	ad.parentWindow = parent
	ad.setupWidgets()
	ad.createDialog(parent)
	ad.dialog.Show()
}

func (ad *AuthDialog) setupWidgets() {
	ad.tokenEntry = widget.NewPasswordEntry()
	ad.tokenEntry.SetPlaceHolder("Enter your API token")
	ad.tokenEntry.OnSubmitted = func(text string) {
		if text != "" {
			ad.authenticate(text)
		}
	}

	ad.loginBtn = widget.NewButtonWithIcon("Login", theme.ConfirmIcon(), func() {
		if ad.tokenEntry.Text != "" {
			ad.authenticate(ad.tokenEntry.Text)
		}
	})
	ad.loginBtn.Importance = widget.HighImportance

	ad.cancelBtn = widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), func() {
		ad.dialog.Hide()
	})

	ad.statusLabel = widget.NewLabel("Enter your API token to authenticate")
	ad.progressBar = widget.NewProgressBar()
	ad.progressBar.Hide()

	helpURL, _ := url.Parse("https://new.akarpov.ru/users/tokens/create/?name=MusicToken&permissions=music.listen&permissions=music.upload&permissions=music.playlist")
	ad.helpLink = widget.NewHyperlink("Get your API token here", helpURL)
}

func (ad *AuthDialog) createDialog(parent fyne.Window) {
	headerIcon := widget.NewIcon(theme.AccountIcon())
	headerLabel := widget.NewLabel("Authentication Required")
	headerLabel.TextStyle = fyne.TextStyle{Bold: true}

	header := container.NewHBox(
		headerIcon,
		headerLabel,
	)
	inputSection := container.NewVBox(
		widget.NewLabel("API Token:"),
		ad.tokenEntry,
		ad.statusLabel,
		ad.progressBar,
	)

	helpSection := container.NewVBox(
		widget.NewSeparator(),
		ad.helpLink,
	)

	buttonSection := container.NewHBox(
		ad.cancelBtn,
		ad.loginBtn,
	)

	content := container.NewVBox(
		header,
		widget.NewSeparator(),
		inputSection,
		helpSection,
		widget.NewSeparator(),
		buttonSection,
	)

	ad.dialog = dialog.NewCustom("Authentication", "", content, parent)
	ad.dialog.Resize(fyne.NewSize(450, 400))
}

func (ad *AuthDialog) authenticate(token string) {
	ad.setUIEnabled(false)
	ad.statusLabel.SetText("Authenticating...")
	ad.progressBar.Show()

	go func() {
		defer func() {
			ad.setUIEnabled(true)
			ad.progressBar.Hide()
		}()

		ctx := context.Background()
		err := ad.api.Authenticate(ctx, token)
		if err != nil {
			log.Printf("Authentication failed: %v", err)
			ad.statusLabel.SetText("Authentication failed: " + err.Error())
			ad.showError(err)
			return
		}

		ad.statusLabel.SetText("Authentication successful!")
		ad.dialog.Hide()

		if ad.onAuthenticated != nil {
			ad.onAuthenticated(token)
		}
	}()
}

func (ad *AuthDialog) setUIEnabled(enabled bool) {
	if enabled {
		ad.tokenEntry.Enable()
		ad.loginBtn.Enable()
	} else {
		ad.tokenEntry.Disable()
		ad.loginBtn.Disable()
	}
}

func (ad *AuthDialog) showError(err error) {
	if ad.parentWindow != nil {
		errorDialog := dialog.NewError(err, ad.parentWindow)
		errorDialog.Show()
	}
}

func (ad *AuthDialog) OnAuthenticated(callback func(string)) {
	ad.onAuthenticated = callback
}
