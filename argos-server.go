package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	unknownAppIcon               = "·"
	impossibleNotificationStatus = "-"
)

type appname string

var appDisplayIcons = map[appname]string{
	"mail.hobsons.com":    "σ",
	"hobsons.slack.com":   "@",
	"hangouts.google.com": "π",
}

var notificationMaxLifetime = time.Minute * 30

type notification struct {
	label     string
	updatedAt time.Time
}

// An ArgosNotificationServer listens for HTTP notifications of the form
// POST /[foo.domain] with a payload containing a {"label":"..."} to
// indicate that a webapp at foo.domain has "..." unread notifications.
//
// Any non-empty string not equal to "0" is considered a notification count,
// and is displayed as-is.
type ArgosNotificationServer struct {
	Host      string
	Port      int
	ArgosHome string

	lastNotificationStatus string

	notificationLock sync.Mutex
	notifications    map[appname]notification
}

// An ArgosNotificationOption sets one or more configuration parameters on an
// *ArgosNotificationServer
type ArgosNotificationOption func(*ArgosNotificationServer)

// ServerHost sets the Argos notification server listen address. If unset, defaults to "localhost"
func ServerHost(host string) ArgosNotificationOption {
	return func(s *ArgosNotificationServer) {
		s.Host = host
	}
}

// ServerPort sets the Argos notification server bind port. This setting is required.
func ServerPort(port int) ArgosNotificationOption {
	return func(s *ArgosNotificationServer) {
		s.Port = port
	}
}

// ArgosHome sets the Argos notification home directory. This setting is required.
func ArgosHome(home string) ArgosNotificationOption {
	return func(s *ArgosNotificationServer) {
		s.ArgosHome = home
	}
}

// NewArgosNotificationServer creates a new HTTP server that translates web app unread notification
// counts into an Argos status badge
func NewArgosNotificationServer(opts ...ArgosNotificationOption) (*ArgosNotificationServer, error) {
	server := &ArgosNotificationServer{}
	for _, opt := range opts {
		opt(server)
	}
	return server, server.init()
}

func (s *ArgosNotificationServer) init() error {
	if s.Port == 0 {
		return errors.New("server port not set")
	}
	if s.ArgosHome == "" {
		return errors.New("Argos home directory not set")
	}
	if s.Host == "" {
		s.Host = "localhost"
	}
	s.notifications = map[appname]notification{}
	s.lastNotificationStatus = impossibleNotificationStatus
	return nil
}

// ListenAddr returns the HTTP bind address for the Argos notification server
func (s *ArgosNotificationServer) ListenAddr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

func (s *ArgosNotificationServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	notifyingApp, err := requestURLNotificationApp(r.URL.Path)
	if err != nil {
		log.Println("Request to invalid path", r.URL.Path, ":", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var notification struct {
		Label string `json:"label"`
	}
	if err = json.NewDecoder(r.Body).Decode(&notification); err != nil {
		log.Println("JSON payload doesn't contain label")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	s.updateArgosStatus(notifyingApp, notification.Label)
	w.WriteHeader(http.StatusOK)
}

func requestURLNotificationApp(path string) (string, error) {
	if len(path) < 2 || !strings.HasPrefix(path, "/") {
		return "", errors.New("no slash prefix")
	}
	return path[1:], nil
}

func (s *ArgosNotificationServer) updateArgosStatus(app, notifications string) {
	s.notificationLock.Lock()
	defer s.notificationLock.Unlock()

	s.notifications[appname(app)] = notification{
		label:     notifications,
		updatedAt: time.Now(),
	}
}

// PushArgosStatus periodically writes the notification status to
// [ArgosHome]/.notification.status
func (s *ArgosNotificationServer) PushArgosStatus() {
	for {
		s.pruneStaleNotifications()
		if err := s.writeNotificationStatus(); err != nil {
			log.Println("error writing notification status:", err)
		}
		time.Sleep(4700 * time.Millisecond)
	}
}

func (s *ArgosNotificationServer) argosNotificationFilepath() string {
	return filepath.Join(s.ArgosHome, ".notifications")
}

func (s *ArgosNotificationServer) argosNotificationTempFilepath() string {
	return filepath.Join(s.ArgosHome, ".notifications.tmp")
}

func (s *ArgosNotificationServer) writeNotificationStatus() error {
	notificationStatus := s.NotificationStatus()
	if notificationStatus == s.lastNotificationStatus {
		return nil
	}
	s.lastNotificationStatus = notificationStatus

	tempFilePath, err := s.writeNotificationTempFile(notificationStatus)
	if err != nil {
		return err
	}
	return os.Rename(tempFilePath, s.argosNotificationFilepath())
}

func (s *ArgosNotificationServer) writeNotificationTempFile(status string) (tmpFilePath string, err error) {
	tmpFilePath = s.argosNotificationTempFilepath()
	statusFH, err := os.Create(tmpFilePath)
	if err != nil {
		return tmpFilePath, err
	}
	defer statusFH.Close()

	_, err = fmt.Fprintln(statusFH, status)
	return tmpFilePath, err
}

// NotificationStatus returns the current notification status
func (s *ArgosNotificationServer) NotificationStatus() string {
	s.notificationLock.Lock()
	defer s.notificationLock.Unlock()

	var visibleNotifications []string
	for app, notifications := range s.notifications {
		if appDisplay := s.notificationDisplay(app, notifications); appDisplay != "" {
			visibleNotifications = append(visibleNotifications, appDisplay)
		}
	}
	sort.Strings(visibleNotifications)
	return strings.Join(visibleNotifications, " ")
}

func (s *ArgosNotificationServer) notificationDisplay(app appname, notifications notification) string {
	if notifications.label == "" || notifications.label == "0" {
		return ""
	}

	appIcon := appDisplayIcons[app]
	if appIcon == "" {
		appIcon = unknownAppIcon
	}
	if notifications.label == "1" {
		return appIcon
	}
	return fmt.Sprint(appIcon, notifications.label)
}

func (s *ArgosNotificationServer) pruneStaleNotifications() {
	s.notificationLock.Lock()
	defer s.notificationLock.Unlock()

	var defunctApps []appname
	for app, notifications := range s.notifications {
		if time.Since(notifications.updatedAt) > notificationMaxLifetime {
			defunctApps = append(defunctApps, app)
		}
	}

	for _, app := range defunctApps {
		delete(s.notifications, app)
	}
}
