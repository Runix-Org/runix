// Package tray owns the native Qt tray icon and its menu on the GUI thread.
package tray

import (
	"fmt"
	"os"
	"runtime"

	qt "github.com/mappu/miqt/qt6"
)

// Install retains one registration, even without a host so Qt can register later.
// Request must submit launcher work without blocking the GUI thread.
// Call the returned cleanup on the GUI thread before destroying the application.
func Install(request func(string) bool) func() {
	return install(request, "runix UI lab", "toggle", []menuAction{
		{"Show", "show"}, {"Hide", "hide"}, {"Show / Hide", "toggle"},
	})
}

// InstallProduct installs the product's Show/Quit menu with click-to-show activation.
// It has the same GUI-thread and cleanup requirements as Install.
func InstallProduct(request func(string) bool) func() {
	return install(request, "runix", "show", []menuAction{{"Show", "show"}})
}

type menuAction struct{ label, command string }

func install(request func(string) bool, tooltip, clickCommand string, actions []menuAction) func() {
	// A built-in raster icon does not depend on a desktop theme or SVG plugin.
	image := qt.NewQImage3(32, 32, qt.QImage__Format_ARGB32)
	image.Fill(0xff3878d4)
	for y := 7; y < 25; y++ {
		for x := 8; x < 24; x++ {
			if x < 12 || y < 11 || y >= 15 && y < 19 || x-y >= 0 && x-y <= 3 {
				image.SetPixel(x, y, 0xffffffff)
			}
		}
	}
	pixmap := qt.QPixmap_FromImage(image)
	runtime.SetFinalizer(pixmap, nil)
	icon := qt.NewQIcon2(pixmap)
	entry := qt.NewQSystemTrayIcon2(icon)
	icon.Delete()
	pixmap.Delete()
	image.Delete()
	menu := qt.NewQMenu2()
	for _, action := range actions {
		menu.AddActionWithText(action.label).OnTriggered(func() {
			if !request(action.command) {
				fmt.Fprintln(os.Stderr, "The launcher is busy or stopping.")
			}
		})
	}
	menu.AddSeparator()
	menu.AddActionWithText("Quit").OnTriggered(qt.QCoreApplication_Quit)
	entry.SetContextMenu(menu)
	entry.SetToolTip(tooltip)
	entry.OnActivated(func(reason qt.QSystemTrayIcon__ActivationReason) {
		if reason == qt.QSystemTrayIcon__Trigger {
			if !request(clickCommand) {
				fmt.Fprintln(os.Stderr, "The launcher is busy or stopping.")
			}
		}
	})
	if !qt.QSystemTrayIcon_IsSystemTrayAvailable() {
		fmt.Fprintln(os.Stderr, "The system tray is not available. Use the activation socket to show the window.")
	}
	entry.Show()
	return func() {
		if entry == nil {
			return
		}
		entry.Hide()
		entry.Delete()
		menu.Delete()
		entry = nil
	}
}
