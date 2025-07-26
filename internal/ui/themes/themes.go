package themes

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type AMPTheme struct {
	variant string
}

func NewTheme(variant string) fyne.Theme {
	return &AMPTheme{variant: variant}
}

func (t *AMPTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if t.variant == "dark" {
		return t.colorDark(name)
	}
	return t.colorLight(name)
}

func (t *AMPTheme) colorDark(name fyne.ThemeColorName) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 18, G: 18, B: 20, A: 255}
	case theme.ColorNameButton:
		return color.NRGBA{R: 45, G: 45, B: 50, A: 255}
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 30, G: 30, B: 35, A: 255}
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 80, G: 80, B: 85, A: 255}
	case theme.ColorNameError:
		return color.NRGBA{R: 255, G: 85, B: 85, A: 255}
	case theme.ColorNameFocus:
		return color.NRGBA{R: 100, G: 150, B: 255, A: 255}
	case theme.ColorNameForeground:
		return color.NRGBA{R: 250, G: 250, B: 252, A: 255}
	case theme.ColorNameHover:
		return color.NRGBA{R: 55, G: 55, B: 65, A: 255}
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 35, G: 35, B: 40, A: 255}
	case theme.ColorNameInputBorder:
		return color.NRGBA{R: 65, G: 65, B: 75, A: 255}
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 25, G: 25, B: 30, A: 255}
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 200}
	case theme.ColorNamePressed:
		return color.NRGBA{R: 75, G: 75, B: 85, A: 255}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 120, G: 160, B: 255, A: 255}
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 85, G: 85, B: 95, A: 255}
	case theme.ColorNameSelection:
		return color.NRGBA{R: 120, G: 160, B: 255, A: 80}
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 150}
	case theme.ColorNameSuccess:
		return color.NRGBA{R: 80, G: 200, B: 120, A: 255}
	case theme.ColorNameWarning:
		return color.NRGBA{R: 255, G: 200, B: 50, A: 255}
	case theme.ColorNameHyperlink:
		return color.NRGBA{R: 150, G: 180, B: 255, A: 255}
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 120, G: 120, B: 130, A: 255}
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 60, G: 60, B: 70, A: 255}
	default:
		return theme.DefaultTheme().Color(name, theme.VariantDark)
	}
}

func (t *AMPTheme) colorLight(name fyne.ThemeColorName) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 248, G: 249, B: 250, A: 255}
	case theme.ColorNameButton:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 240, G: 242, B: 245, A: 255}
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 140, G: 150, B: 160, A: 255}
	case theme.ColorNameError:
		return color.NRGBA{R: 220, G: 53, B: 69, A: 255}
	case theme.ColorNameFocus:
		return color.NRGBA{R: 0, G: 123, B: 255, A: 255}
	case theme.ColorNameForeground:
		return color.NRGBA{R: 33, G: 37, B: 41, A: 255}
	case theme.ColorNameHover:
		return color.NRGBA{R: 240, G: 242, B: 245, A: 255}
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	case theme.ColorNameInputBorder:
		return color.NRGBA{R: 206, G: 212, B: 218, A: 255}
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 220}
	case theme.ColorNamePressed:
		return color.NRGBA{R: 220, G: 225, B: 230, A: 255}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 0, G: 123, B: 255, A: 255}
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 180, G: 188, B: 196, A: 255}
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0, G: 123, B: 255, A: 60}
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 60}
	case theme.ColorNameSuccess:
		return color.NRGBA{R: 40, G: 167, B: 69, A: 255}
	case theme.ColorNameWarning:
		return color.NRGBA{R: 255, G: 165, B: 0, A: 255}
	case theme.ColorNameHyperlink:
		return color.NRGBA{R: 0, G: 100, B: 200, A: 255}
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 134, G: 142, B: 150, A: 255}
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 218, G: 220, B: 224, A: 255}
	default:
		return theme.DefaultTheme().Color(name, theme.VariantLight)
	}
}

func (t *AMPTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *AMPTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *AMPTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 8
	case theme.SizeNameScrollBar:
		return 14
	case theme.SizeNameScrollBarSmall:
		return 10
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameText:
		return 14
	case theme.SizeNameCaptionText:
		return 12
	case theme.SizeNameInputBorder:
		return 1
	case theme.SizeNameInnerPadding:
		return 6
	default:
		return theme.DefaultTheme().Size(name)
	}
}
