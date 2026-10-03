package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"mediaforge/internal/sabnzbd"
	"net/http"
)

func (s *Server) sabConfig() *sabnzbd.Client {
	return sabnzbd.New(s.directClient, s.cfg.SABnzbdURL, s.cfg.SABnzbdKey)
}
func (s *Server) handleListUsenetServers(w http.ResponseWriter, r *http.Request) {
	servers, err := s.sabConfig().Servers(r.Context())
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, servers)
}
func (s *Server) handleSaveUsenetServer(w http.ResponseWriter, r *http.Request) {
	var req sabnzbd.Server
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}
	client := s.sabConfig()
	if r.Method == "POST" {
		var id [12]byte
		if _, err := rand.Read(id[:]); err != nil {
			writeError(w, 500, "generate server ID failed")
			return
		}
		req.ID = "zarr-" + hex.EncodeToString(id[:])
	} else {
		req.ID = r.PathValue("id")
		servers, err := client.Servers(r.Context())
		if err != nil {
			writeError(w, 502, err.Error())
			return
		}
		found := false
		for _, existing := range servers {
			if existing.ID == req.ID {
				found = true
				break
			}
		}
		if !found {
			writeError(w, 404, "Usenet server not found")
			return
		}
	}
	if err := req.Validate(); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if err := client.Save(r.Context(), req); err != nil {
		writeError(w, 502, err.Error())
		return
	}
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	writeJSON(w, status, map[string]string{"id": req.ID})
}
func (s *Server) handleDeleteUsenetServer(w http.ResponseWriter, r *http.Request) {
	if err := s.sabConfig().Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}
func (s *Server) handleTestUsenetServer(w http.ResponseWriter, r *http.Request) {
	client := s.sabConfig()
	servers, err := client.Servers(r.Context())
	if err != nil {
		testResult(w, err)
		return
	}
	for _, server := range servers {
		if server.ID == r.PathValue("id") {
			testResult(w, client.TestServer(r.Context(), server))
			return
		}
	}
	writeError(w, 404, "Usenet server not found")
}
