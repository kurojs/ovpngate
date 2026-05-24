package favstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Favorite struct {
	IP           string `json:"ip"`
	HostName     string `json:"hostname"`
	CountryShort string `json:"countryShort"`
	CountryLong  string `json:"countryLong"`
	Alias        string `json:"alias,omitempty"`
	SavedAt      string `json:"savedAt"`
}

type Store struct {
	path string
	mu   sync.Mutex
	favs map[string]*Favorite
}

func DefaultPath() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cfg, "ovpngate")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "favorites.json"), nil
}

func New(path string) *Store {
	return &Store{
		path: path,
		favs: make(map[string]*Favorite),
	}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var list []*Favorite
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	s.favs = make(map[string]*Favorite, len(list))
	for _, f := range list {
		s.favs[f.IP] = f
	}
	return nil
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	list := make([]*Favorite, 0, len(s.favs))
	for _, f := range s.favs {
		list = append(list, f)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (s *Store) Add(ip, hostname, countryShort, countryLong string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.favs[ip] = &Favorite{
		IP:           ip,
		HostName:     hostname,
		CountryShort: countryShort,
		CountryLong:  countryLong,
		SavedAt:      time.Now().Format(time.RFC3339),
	}
}

func (s *Store) Remove(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.favs, ip)
}

func (s *Store) Rename(ip, alias string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if f, ok := s.favs[ip]; ok {
		f.Alias = alias
	}
}

func (s *Store) IsFavorite(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.favs[ip]
	return ok
}

func (s *Store) GetAll() []*Favorite {
	s.mu.Lock()
	defer s.mu.Unlock()

	list := make([]*Favorite, 0, len(s.favs))
	for _, f := range s.favs {
		list = append(list, f)
	}
	return list
}

func (s *Store) Get(ip string) *Favorite {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.favs[ip]
}
