package services

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var DefaultUserDirectories = []string{"backup", "logs", "data", "htdocs", "config"}

func ProvisionUserHome(home string, uid, gid int) error {
	if err := os.MkdirAll(home, 0755); err != nil {
		return err
	}
	if err := os.Chown(home, uid, gid); err != nil {
		return err
	}

	for _, name := range DefaultUserDirectories {
		directory := filepath.Join(home, name)
		if err := os.MkdirAll(directory, 0755); err != nil {
			return err
		}
		if err := os.Chown(directory, uid, gid); err != nil {
			return err
		}
	}

	return nil
}

type LinuxUser struct {
	Home     string `json:"home"`
	Name     string `json:"name"`
	Shell    string `json:"shell"`
	UID      int    `json:"uid"`
	Username string `json:"username"`
}

func HomeUsers() ([]LinuxUser, error) {
	return homeUsersIn("/home", passwdUsersByName())
}

func homeUsersIn(homeRoot string, passwdUsers map[string]passwdUser) ([]LinuxUser, error) {
	entries, err := os.ReadDir(homeRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []LinuxUser{}, nil
		}
		return nil, err
	}

	users := make([]LinuxUser, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		home := filepath.Join(homeRoot, name)
		linuxUser := LinuxUser{
			Home:     home,
			Name:     name,
			Shell:    passwdUsers[name].Shell,
			UID:      -1,
			Username: name,
		}

		if passwdUsers[name].Username != "" {
			linuxUser.Username = passwdUsers[name].Username
			linuxUser.UID = passwdUsers[name].UID
		}

		users = append(users, linuxUser)
	}

	return users, nil
}

type passwdUser struct {
	Shell    string
	UID      int
	Username string
}

func passwdUsersByName() map[string]passwdUser {
	file, err := os.Open("/etc/passwd")
	if err != nil {
		return map[string]passwdUser{}
	}
	defer file.Close()

	users := make(map[string]passwdUser)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ":")
		if len(parts) < 7 {
			continue
		}

		uid, _ := strconv.Atoi(parts[2])
		users[parts[0]] = passwdUser{
			Shell:    parts[6],
			UID:      uid,
			Username: parts[0],
		}
	}

	return users
}
