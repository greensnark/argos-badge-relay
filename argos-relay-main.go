package main

import (
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func main() {
	bindEnvironmentVars(argosRelayCommand()).Execute()
}

func argosRelayCommand() *cobra.Command {
	c := &cobra.Command{
		Short: "Listens for HTTP POST requests reporting unread-notifications in webapps and badges the apps in Argos",
		Run:   listenForUnreadNotifications,
	}

	flags := c.Flags()
	flags.Int("port", 18989, "port to start notification http server on")
	flags.String("host", "localhost", "host interface to start notification server on")
	flags.String("argos-root", expandHomeDir("~/.config/argos"), "Argos notification base directory")
	return c
}

func bindEnvironmentVars(c *cobra.Command) *cobra.Command {
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.SetEnvPrefix("NF")
	viper.AutomaticEnv()
	viper.BindPFlags(c.Flags())
	return c
}

func listenForUnreadNotifications(c *cobra.Command, args []string) {
	notificationServer, err := NewArgosNotificationServer(
		ServerHost(viper.GetString("host")),
		ServerPort(viper.GetInt("port")),
		ArgosHome(viper.GetString("argos-root")))

	if err != nil {
		fmt.Fprintln(os.Stderr, "Unable to start Argos notification relay:", err)
		os.Exit(1)
	}

	go notificationServer.PushArgosStatus()
	if err := http.ListenAndServe(notificationServer.ListenAddr(), notificationServer); err != nil {
		fmt.Fprintln(os.Stderr, "error starting notification server:", err)
		os.Exit(1)
	}
}

// expandHomeDir replaces a leading "~/" in path with the user home directory
func expandHomeDir(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}

	currentUser, err := user.Current()
	if err != nil {
		panic("unable to determine user $HOME")
	}
	return filepath.Join(currentUser.HomeDir, path[2:])
}
