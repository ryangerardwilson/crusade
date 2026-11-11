package tui

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// launchMeetingLink opens a meeting link. Tests replace it.
var launchMeetingLink = defaultLaunchMeetingLink

func (m *calModel) openSelected() bool {
	m.pendingDelete = false
	item, ok := m.selectedItem()
	if !ok {
		m.errText = ""
		m.status = "No block selected to open."
		return false
	}
	link, ok := meetingLink(item.url)
	if !ok {
		if strings.TrimSpace(item.url) == "" {
			m.errText = ""
			m.status = "No link on this block."
			return false
		}
		m.fail(fmt.Errorf("that link cannot be opened"))
		return false
	}
	if err := launchMeetingLink(link); err != nil {
		m.fail(err)
		return false
	}
	m.errText = ""
	m.status = "Opened the link."
	return os.Getenv("CRUSADE_POPUP") == "1"
}

func meetingLink(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Hostname() == "" {
		return "", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false
	}
	return parsed.String(), true
}

func defaultLaunchMeetingLink(link string) error {
	command, err := meetingBrowser()
	if err != nil {
		return err
	}
	cmd := exec.Command(command, link)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not open the link")
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func meetingBrowser() (string, error) {
	if path, err := exec.LookPath("omarchy-launch-browser"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("xdg-open"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("could not open the link")
}
