package ui

import "github.com/egoist/mygo/ui"

const configureSigning commandID = "settings.configure-gpg-key"

func (a *Workspace) configureSigningSettings() {
	a.openSettings()
	a.settings.page = "Git"
	a.settings.signingOpen = true
}

func (a *Workspace) signingSettings(c *ui.Context, p tokens) {
	git := &a.settings.vcs.Git
	ui.Button(c, "Configure GPG Key…").Width(160).Label(commandLabel(configureSigning)).OnClick(func() { a.run(configureSigning, commandTarget{}) })
	if !a.settings.signingOpen {
		label := "Commit/Amend signing follows Git configuration"
		if git.SignCommits {
			label = "Commit/Amend OpenPGP key: " + git.SigningPolicy().KeyID()
		}
		ui.Text(c, label).FillWidth().TextColor(p.theme.TextMuted)
		return
	}
	settingsSection(c, p, "OpenPGP Commit Signing")
	ui.Checkbox(c, &git.SignCommits, "Use this OpenPGP key for Vela Commit/Amend")
	ui.TextInput(c, &git.SigningKey).Label("OpenPGP key ID or fingerprint").Placeholder("Full fingerprint recommended").FillWidth().Disabled(!git.SignCommits)
	ui.Text(c, "Accepts a 16-digit key ID or 40/64-digit fingerprint; a trailing ! selects an exact subkey. The key must already be installed for your Git/GPG setup.").FillWidth().TextColor(p.theme.TextMuted)
	ui.Text(c, "When unchecked, Git signing settings remain in effect. Other Git workflows use their Git signing configuration. Apply/OK saves this app preference; no Git config files or private keys are changed.").FillWidth().TextColor(p.theme.TextMuted)
	ui.Text(c, "Git may open the configured GPG passphrase prompt. A signing error stops Commit/Amend; it never retries unsigned.").FillWidth().TextColor(p.theme.TextMuted)
	if err := git.SigningPolicy().Validate(); err != nil {
		ui.Text(c, err.Error()).FillWidth().TextColor(p.theme.Danger)
	}
}
