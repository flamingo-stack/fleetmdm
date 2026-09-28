package externalrefs

import (
	"fmt"

	maintained_apps "github.com/fleetdm/fleet/v4/ee/maintained-apps"
)

var Funcs = map[string][]func(*maintained_apps.FMAManifestApp) (*maintained_apps.FMAManifestApp, error){
	"microsoft-word/darwin":         {MicrosoftVersionFromReleaseNotes},
	"microsoft-excel/darwin":        {MicrosoftVersionFromReleaseNotes},
	"microsoft-outlook/darwin":      {MicrosoftVersionFromReleaseNotes},
	"microsoft-powerpoint/darwin":   {MicrosoftVersionFromReleaseNotes},
	"microsoft-onenote/darwin":      {MicrosoftVersionFromReleaseNotes},
	"brave-browser/darwin":          {BraveVersionTransformer},
	"whatsapp/darwin":               {WhatsAppVersionShortener, WhatsAppInstallerURLOverride("https://web.whatsapp.com/desktop/mac_native/release/?configuration=Release&src=whatsapp_downloads_page")},
	"google-chrome/darwin":          {PKGInstallerOverride("https://dl.google.com/dl/chrome/mac/universal/stable/gcem/GoogleChrome.pkg")},
	"google-drive/darwin":           {GoogleDriveVersionShortener},
	"1password/darwin":              {PKGInstallerOverride("https://downloads.1password.com/mac/1Password.pkg")},
	"zoom/darwin":                   {PKGInstallerOverride("https://zoom.us/client/latest/ZoomInstallerIT.pkg")},
	"slack/darwin":                  {PKGInstallerOverride("https://slack.com/api/desktop.latestRelease?redirect=1&variant=pkg&arch=universal")},
	"omnissa-horizon-client/darwin": {OmnissaHorizonVersionShortener},
	"8x8-work/darwin":               {EightXEightWorkVersionShortener},
	"cisco-jabber/darwin":           {CiscoJabberVersionTransformer},
	"parallels/darwin":              {ParallelsVersionShortener},
	"github/darwin":                 {GitHubDesktopVersionShortener},
	"camtasia/darwin":               {CamtasiaVersionTransformer},
	"warp/darwin":                   {WarpDirectInstaller},
	"android-studio/darwin":         {AndroidStudioVersionShortener},
	"microsoft-auto-update/darwin":  {MicrosoftAutoUpdateVersionShortener},
	"opera/darwin":                  {OperaVersionShortener},
	"twingate/darwin":               {TwingateVersionShortener},
	"citrix-workspace/darwin":       {CitrixWorkspaceVersionShortener},
	"elgato-stream-deck/darwin":     {ElgatoStreamDeckVersionShortener},
	"filemaker-pro/darwin":          {FileMakerProVersionShortener},
	"royal-tsx/darwin":              {RoyalTSXVersionShortener},
	"sublime-text/darwin":           {SublimeVersionTransformer},
	"sublime-merge/darwin":          {SublimeVersionTransformer},
	"mysqlworkbench/darwin":         {MySQLWorkbenchVersionTransformer},
	"lens/darwin":                   {LensVersionTransformer},
	"grammarly-desktop/darwin":      {GrammarlyDesktopVersionShortener},
	"logitune/darwin":               {PKGInstallerOverride("https://software.vc.logitech.com/downloads/tune/LogiTuneInstaller.pkg")},
	"anka-virtualization/darwin":    {AnkaVersionShortener},
	"pd/darwin":                     {PdVersionTransformer},
	"sonos/darwin":                  {SonosVersionTransformer},
}

// PKGInstallerOverride returns an enricher function that overrides the installer URL
// to use the given PKG installer URL instead of Homebrew's default (typically a DMG).
// Version is kept from Homebrew (not set to "latest"). SHA256 is set to "no_check"
// since the installer URL differs from Homebrew's.
func PKGInstallerOverride(url string) func(*maintained_apps.FMAManifestApp) (*maintained_apps.FMAManifestApp, error) {
	return func(app *maintained_apps.FMAManifestApp) (*maintained_apps.FMAManifestApp, error) {
		app.InstallerURL = url
		app.SHA256 = "no_check"

		return app, nil
	}
}

// WhatsAppInstallerURLOverride returns an enricher function that overrides the installer URL
// to always use the given URL. SHA256 is set to "no_check" since the installer URL differs
// from Homebrew's.
func WhatsAppInstallerURLOverride(url string) func(*maintained_apps.FMAManifestApp) (*maintained_apps.FMAManifestApp, error) {
	return func(app *maintained_apps.FMAManifestApp) (*maintained_apps.FMAManifestApp, error) {
		app.InstallerURL = url
		app.SHA256 = "no_check"

		return app, nil
	}
}

func EnrichManifest(app *maintained_apps.FMAManifestApp) {
	// Enrich the app manifest with additional metadata
	if enrichers, ok := Funcs[app.Slug]; ok {
		for _, enricher := range enrichers {
			var err error
			app, err = enricher(app)
			if err != nil {
				fmt.Printf("Error enriching app %s: %v\n", app.UniqueIdentifier, err)
			}
		}
	}
}
