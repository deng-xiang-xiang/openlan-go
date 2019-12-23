package service

import (
	"bufio"
	"github.com/danieldin95/openlan-go/models"
	"os"
	"strings"
	"sync"
)

type _user struct {
	lock   sync.RWMutex
	_users map[string]*models.User
}

var User = _user{
	_users: make(map[string]*models.User, 1024),
}

func (w *_user) Load(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}

	defer file.Close()
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		values := strings.Split(line, ":")
		if len(values) == 2 {
			_user := models.NewUser(values[0], strings.TrimSpace(values[1]))
			w.Add(_user)
		}
	}

	return nil
}

func (w *_user) Add(_user *models.User) {
	w.lock.Lock()
	defer w.lock.Unlock()

	name := _user.Name
	if name == "" {
		name = _user.Token
	}
	w._users[name] = _user
}

func (w *_user) Del(name string) {
	w.lock.Lock()
	defer w.lock.Unlock()

	if _, ok := w._users[name]; ok {
		delete(w._users, name)
	}
}

func (w *_user) Get(name string) *models.User {
	w.lock.RLock()
	defer w.lock.RUnlock()

	if u, ok := w._users[name]; ok {
		return u
	}

	return nil
}

func (w *_user) List() <-chan *models.User {
	c := make(chan *models.User, 128)

	go func() {
		w.lock.RLock()
		defer w.lock.RUnlock()

		for _, u := range w._users {
			c <- u
		}
		c <- nil //Finish channel by nil.
	}()

	return c
}