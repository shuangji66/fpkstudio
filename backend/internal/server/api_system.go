package server

import (
	"net/http"
	"os"
)

type fnpackInfo struct {
	Bin     string `json:"bin"`
	Version string `json:"version"`
	Message string `json:"message"`
	OK      bool   `json:"ok"`
}

type systemInfo struct {
	Version   string     `json:"version"`
	BaseURL   string     `json:"baseURL"`
	Socket    string     `json:"socket"`
	PID       int        `json:"pid"`
	PIDFile   string     `json:"pidFile"`
	DataDir   string     `json:"dataDir"`
	Projects  string     `json:"projectsDir"`
	OutputDir string     `json:"outputDir"`
	LogDir    string     `json:"logDir"`
	TmpDir    string     `json:"tmpDir"`
	LogFile   string     `json:"logFile"`
	Fnpack    fnpackInfo `json:"fnpack"`
	LastSeq   int64      `json:"lastSeq"`
	StartedAt string     `json:"startedAt"`
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) error {
	bin, ver, msg := s.fn.Probe(r.Context())
	return writeJSON(w, systemInfo{
		Version:   s.cfg.Version,
		BaseURL:   s.cfg.BaseURL,
		Socket:    s.cfg.SocketPath,
		PID:       os.Getpid(),
		PIDFile:   s.cfg.PIDFile,
		DataDir:   s.cfg.DataDir,
		Projects:  s.prj.ProjectRoot(),
		OutputDir: s.prj.OutputDir(),
		LogDir:    s.cfg.LogDir,
		TmpDir:    s.cfg.TmpDir,
		LogFile:   s.cfg.LogFile(),
		Fnpack: fnpackInfo{
			Bin:     bin,
			Version: ver,
			Message: msg,
			OK:      msg == "",
		},
		LastSeq:   s.log.LastSeq(),
		StartedAt: s.startedAt.Format("2006-01-02 15:04:05"),
	})
}

func (s *Server) handleSystemFnpack(w http.ResponseWriter, r *http.Request) error {
	bin, ver, msg := s.fn.Probe(r.Context())
	return writeJSON(w, fnpackInfo{Bin: bin, Version: ver, Message: msg, OK: msg == ""})
}
