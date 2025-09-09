package main

import "sync"

type requestRec struct {
	Time       string              `json:"time"`
	Method     string              `json:"method"`
	Path       string              `json:"path"`
	Query      string              `json:"query"`
	Proto      string              `json:"proto"`
	RemoteAddr string              `json:"remoteAddr"`
	Host       string              `json:"host"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body"`
	BodyBytes  int                 `json:"bodyBytes"`
	Truncated  bool                `json:"truncated"`
}

var (
	histMu  sync.Mutex
	histBuf []requestRec
	histIdx int
)

func appendHistory(rec requestRec) {
	histMu.Lock()
	defer histMu.Unlock()
	if historySize <= 0 {
		return
	}
	if histBuf == nil {
		histBuf = make([]requestRec, 0, historySize)
	}
	if len(histBuf) < historySize {
		histBuf = append(histBuf, rec)
		return
	}
	// ring overwrite at histIdx
	histBuf[histIdx] = rec
	histIdx = (histIdx + 1) % historySize
}

func snapshotHistory() []requestRec {
	histMu.Lock()
	defer histMu.Unlock()
	if len(histBuf) == 0 {
		return nil
	}
	// produce slice in chronological order (oldest first)
	if len(histBuf) < historySize || histIdx == 0 {
		out := make([]requestRec, len(histBuf))
		copy(out, histBuf)
		return out
	}
	out := make([]requestRec, len(histBuf))
	copy(out, histBuf[histIdx:])
	copy(out[len(histBuf)-histIdx:], histBuf[:histIdx])
	return out
}
