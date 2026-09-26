package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/erkanvatan/pause-together/internal/library"
	"github.com/erkanvatan/pause-together/internal/media"
)

// adminRoutes registers the admin API. Only the admin listener calls it, so on the guest port these
// paths fall through to the /api/ 404.
func (s *server) adminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/libraries", s.listLibraries)
	mux.HandleFunc("POST /api/admin/libraries", s.addLibrary)
	mux.HandleFunc("DELETE /api/admin/libraries/{id}", s.removeLibrary)
	mux.HandleFunc("POST /api/admin/libraries/{id}/scan", s.scanLibrary)
	mux.HandleFunc("GET /api/admin/folders", s.listFolders)
	mux.HandleFunc("GET /api/admin/problems", s.listProblems)
	mux.HandleFunc("GET /api/admin/jobs", s.listJobs)
	mux.HandleFunc("PUT /api/admin/languages", s.putLanguages)
}

// Error codes the admin page turns into text.
const (
	errBadRequest = "bad-request"
	errBadType    = "bad-type"
	errBadLang    = "bad-lang"
	errNotFolder  = "not-folder"
	errOverlap    = "overlap"
)

// writeError sends {"error": code}.
func writeError(w http.ResponseWriter, status int, code string) {
	writeJSONStatus(w, status, map[string]string{"error": code})
}

type libraryResponse struct {
	library.LibraryInfo
	Scan library.ScanStatus `json:"scan"`
}

func (s *server) listLibraries(w http.ResponseWriter, r *http.Request) {
	libs, err := s.Libraries.List(r.Context())
	if err != nil {
		internalError(w, "list libraries", err)
		return
	}
	status := s.Scans.Status()
	resp := make([]libraryResponse, len(libs))
	for i, l := range libs {
		resp[i] = libraryResponse{LibraryInfo: l, Scan: status[l.ID]}
	}
	writeJSON(w, resp)
}

func (s *server) addLibrary(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string       `json:"path"`
		Type library.Type `json:"type"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errBadRequest)
		return
	}
	lib, err := s.Libraries.Add(r.Context(), req.Path, req.Type)
	switch {
	case errors.Is(err, library.ErrBadType):
		writeError(w, http.StatusBadRequest, errBadType)
	case errors.Is(err, library.ErrBadPath), errors.Is(err, library.ErrNotFolder):
		writeError(w, http.StatusBadRequest, errNotFolder)
	case errors.Is(err, library.ErrOverlap):
		writeError(w, http.StatusConflict, errOverlap)
	case err != nil:
		internalError(w, "add library", err)
	default:
		s.Scans.Request(lib.ID)
		writeJSONStatus(w, http.StatusCreated, lib)
	}
}

func (s *server) removeLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch err := s.Libraries.Remove(r.Context(), id); {
	case errors.Is(err, library.ErrNotFound):
		http.NotFound(w, r)
	case err != nil:
		internalError(w, "remove library", err)
	default:
		s.Scans.Removed(id)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *server) scanLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch _, err := s.Libraries.Get(r.Context(), id); {
	case errors.Is(err, library.ErrNotFound):
		http.NotFound(w, r)
	case err != nil:
		internalError(w, "get library", err)
	default:
		s.Scans.Request(id)
		w.WriteHeader(http.StatusAccepted)
	}
}

// listFolders lists the folders inside ?path= (relative to the media folder) for the folder picker.
func (s *server) listFolders(w http.ResponseWriter, r *http.Request) {
	folders, err := s.Libraries.Folders(r.URL.Query().Get("path"))
	switch {
	case errors.Is(err, library.ErrBadPath):
		writeError(w, http.StatusBadRequest, errNotFolder)
	case errors.Is(err, library.ErrNotFolder):
		writeError(w, http.StatusNotFound, errNotFolder)
	case err != nil:
		internalError(w, "list folders", err)
	default:
		writeJSON(w, map[string][]string{"folders": folders})
	}
}

func (s *server) listProblems(w http.ResponseWriter, r *http.Request) {
	p, err := s.Libraries.Problems(r.Context())
	if err != nil {
		internalError(w, "list problems", err)
		return
	}
	writeJSON(w, p)
}

type jobsResponse struct {
	Jobs       []media.JobStatus `json:"jobs"`
	CacheBytes int64             `json:"cacheBytes"`
	FreeBytes  uint64            `json:"freeBytes"`
}

func (s *server) listJobs(w http.ResponseWriter, _ *http.Request) {
	cache, free, err := s.Jobs.Disk()
	if err != nil {
		internalError(w, "cache size", err)
		return
	}
	writeJSON(w, jobsResponse{Jobs: s.Jobs.List(), CacheBytes: cache, FreeBytes: free})
}

func (s *server) putLanguages(w http.ResponseWriter, r *http.Request) {
	var req library.Languages
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errBadRequest)
		return
	}
	langs, err := s.Libraries.SetLanguages(r.Context(), req)
	switch {
	case errors.Is(err, library.ErrBadLang):
		writeError(w, http.StatusBadRequest, errBadLang)
	case err != nil:
		internalError(w, "save languages", err)
	default:
		writeJSON(w, langs)
	}
}

// pathID reads the {id} path value.
func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil
}
