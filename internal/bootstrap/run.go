package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"

	"github.com/egoist/mygo"
	native "github.com/egoist/mygo/ui"
	"vela/internal/adapters"
	"vela/internal/adapters/gitcli"
	"vela/internal/application"
	"vela/internal/branding"
	"vela/internal/ui"
)

func Run() error {
	dir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("resolve user config directory: %w", err)
	}
	return run(filepath.Join(dir, "vela", "layout.json"))
}

func InitializeRepository(ctx context.Context, directory, branch string) (string, error) {
	return gitcli.New().Initialize(ctx, directory, branch)
}

func run(layoutPath string) error {
	layout, err := adapters.LoadLayout(layoutPath)
	if err != nil {
		log.Printf("Using default layout: %v", err)
	}
	workspace := ui.New(application.New(), layout)
	sessionPath := filepath.Join(filepath.Dir(layoutPath), "workspace.json")
	session, sessionErr := adapters.LoadSession(sessionPath)
	if sessionErr != nil {
		log.Printf("Using default workspace: %v", sessionErr)
	} else {
		workspace.RestoreSession(session)
	}
	initial := workspace.SessionSnapshot()
	var initializer application.RepositoryInitializer = gitcli.New()
	mygo.App.WhenReady(func() {
		window := mygo.NewWindow(mygo.WindowOptions{
			Title: "vela", Width: 1440, Height: 900, MinWidth: 900, MinHeight: 620,
			TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: 40,
			Content: native.View(workspace.View),
		})
		if err := window.SetIcon([]byte(branding.IconPNG)); err != nil {
			log.Printf("Set vela window icon: %v", err)
		}
		workspace.InitializeRepository = func(ctx context.Context, directory, branch string, done func(string, error)) {
			go func() {
				path, err := initializer.Initialize(ctx, directory, branch)
				mygo.RunOnMain(func() { done(path, err); window.Invalidate() })
			}()
		}
		workspace.PickRepositoryDirectory = func(done func(string, error)) {
			go func() {
				dirs, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Directory: true, Title: "Choose Repository Directory"})
				var path string
				if len(dirs) > 0 {
					path = dirs[0]
				}
				mygo.RunOnMain(func() { done(path, err); window.Invalidate() })
			}()
		}
		workspace.CloseWindow = func() { go mygo.RunOnMain(window.Close) }
		window.OnClose(func(e *mygo.CloseEvent) {
			if !workspace.RequestClose() {
				e.PreventDefault()
				window.Invalidate()
			}
		})
	})
	runErr := mygo.App.Run()
	if runErr != nil {
		return fmt.Errorf("run native app: %w", runErr)
	}
	var saveLayout, saveSession error
	if workspace.Layout != layout {
		saveLayout = replaceConfig(layoutPath, err, adapters.ErrInvalidLayout, func() error {
			return adapters.SaveLayout(layoutPath, workspace.Layout)
		})
	}
	current := workspace.SessionSnapshot()
	if !reflect.DeepEqual(initial, current) {
		saveSession = replaceConfig(sessionPath, sessionErr, adapters.ErrInvalidSession, func() error {
			return adapters.SaveSession(sessionPath, current)
		})
	}
	return errors.Join(saveLayout, saveSession)
}

func replaceConfig(path string, loadErr, invalid error, save func() error) error {
	if loadErr != nil {
		if !errors.Is(loadErr, invalid) {
			return loadErr
		}
		backup, err := adapters.BackupLayout(path)
		if err != nil {
			return err
		}
		log.Printf("Backed up invalid config to %s", backup)
	}
	return save()
}
