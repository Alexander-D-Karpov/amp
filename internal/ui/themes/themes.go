package themes

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type AMPTheme struct {
	variant string
}

var _ fyne.Theme = (*AMPTheme)(nil)

func NewTheme(variant string) fyne.Theme {
	return &AMPTheme{variant: variant}
}

func (t *AMPTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if t.variant == "dark" {
		return t.colorDark(name, variant)
	}
	return t.colorLight(name, variant)
}

func (t *AMPTheme) colorDark(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	colors := map[fyne.ThemeColorName]color.NRGBA{
		theme.ColorNameBackground:        {R: 18, G: 18, B: 20, A: 255},
		theme.ColorNameButton:            {R: 45, G: 45, B: 50, A: 255},
		theme.ColorNameDisabledButton:    {R: 30, G: 30, B: 35, A: 255},
		theme.ColorNameDisabled:          {R: 80, G: 80, B: 85, A: 255},
		theme.ColorNameError:             {R: 255, G: 85, B: 85, A: 255},
		theme.ColorNameFocus:             {R: 100, G: 150, B: 255, A: 255},
		theme.ColorNameForeground:        {R: 250, G: 250, B: 252, A: 255},
		theme.ColorNameHover:             {R: 55, G: 55, B: 65, A: 255},
		theme.ColorNameInputBackground:   {R: 35, G: 35, B: 40, A: 255},
		theme.ColorNameInputBorder:       {R: 65, G: 65, B: 75, A: 255},
		theme.ColorNameMenuBackground:    {R: 25, G: 25, B: 30, A: 255},
		theme.ColorNameOverlayBackground: {R: 0, G: 0, B: 0, A: 200},
		theme.ColorNamePressed:           {R: 75, G: 75, B: 85, A: 255},
		theme.ColorNamePrimary:           {R: 120, G: 160, B: 255, A: 255},
		theme.ColorNameScrollBar:         {R: 85, G: 85, B: 95, A: 255},
		theme.ColorNameSelection:         {R: 120, G: 160, B: 255, A: 80},
		theme.ColorNameShadow:            {R: 0, G: 0, B: 0, A: 150},
		theme.ColorNameSuccess:           {R: 80, G: 200, B: 120, A: 255},
		theme.ColorNameWarning:           {R: 255, G: 200, B: 50, A: 255},
		theme.ColorNameHyperlink:         {R: 150, G: 180, B: 255, A: 255},
		theme.ColorNamePlaceHolder:       {R: 120, G: 120, B: 130, A: 255},
		theme.ColorNameSeparator:         {R: 60, G: 60, B: 70, A: 255},
	}

	if color, exists := colors[name]; exists {
		return color
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (t *AMPTheme) colorLight(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	colors := map[fyne.ThemeColorName]color.NRGBA{
		theme.ColorNameBackground:        {R: 248, G: 249, B: 250, A: 255},
		theme.ColorNameButton:            {R: 255, G: 255, B: 255, A: 255},
		theme.ColorNameDisabledButton:    {R: 240, G: 242, B: 245, A: 255},
		theme.ColorNameDisabled:          {R: 140, G: 150, B: 160, A: 255},
		theme.ColorNameError:             {R: 220, G: 53, B: 69, A: 255},
		theme.ColorNameFocus:             {R: 0, G: 123, B: 255, A: 255},
		theme.ColorNameForeground:        {R: 33, G: 37, B: 41, A: 255},
		theme.ColorNameHover:             {R: 240, G: 242, B: 245, A: 255},
		theme.ColorNameInputBackground:   {R: 255, G: 255, B: 255, A: 255},
		theme.ColorNameInputBorder:       {R: 206, G: 212, B: 218, A: 255},
		theme.ColorNameMenuBackground:    {R: 255, G: 255, B: 255, A: 255},
		theme.ColorNameOverlayBackground: {R: 255, G: 255, B: 255, A: 220},
		theme.ColorNamePressed:           {R: 220, G: 225, B: 230, A: 255},
		theme.ColorNamePrimary:           {R: 0, G: 123, B: 255, A: 255},
		theme.ColorNameScrollBar:         {R: 180, G: 188, B: 196, A: 255},
		theme.ColorNameSelection:         {R: 0, G: 123, B: 255, A: 60},
		theme.ColorNameShadow:            {R: 0, G: 0, B: 0, A: 60},
		theme.ColorNameSuccess:           {R: 40, G: 167, B: 69, A: 255},
		theme.ColorNameWarning:           {R: 255, G: 165, B: 0, A: 255},
		theme.ColorNameHyperlink:         {R: 0, G: 100, B: 200, A: 255},
		theme.ColorNamePlaceHolder:       {R: 134, G: 142, B: 150, A: 255},
		theme.ColorNameSeparator:         {R: 218, G: 220, B: 224, A: 255},
	}

	if color, exists := colors[name]; exists {
		return color
	}
	return theme.DefaultTheme().Color(name, variant)
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
