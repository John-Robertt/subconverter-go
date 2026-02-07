package httpapi

import (
	"net/http"
)

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	WriteText(w, http.StatusOK, "ok\n")
}

type convertHandler struct {
	opt Options
}

func (h convertHandler) handleSub(w http.ResponseWriter, r *http.Request) {
	req, err := parseConvertGET(r)
	if err != nil {
		writeErrorFromErr(w, err)
		return
	}

	if err := setAttachmentHeaders(w, req); err != nil {
		writeErrorFromErr(w, err)
		return
	}

	out, err := runConvert(r.Context(), r, req, h.opt)
	if err != nil {
		writeErrorFromErr(w, err)
		return
	}
	WriteText(w, http.StatusOK, out)
}

func (h convertHandler) handleConvert(w http.ResponseWriter, r *http.Request) {
	// Prevent abusive payload sizes.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20 /* 1 MiB */)

	req, err := parseConvertPOST(r)
	if err != nil {
		writeErrorFromErr(w, err)
		return
	}

	if err := setAttachmentHeaders(w, req); err != nil {
		writeErrorFromErr(w, err)
		return
	}

	out, err := runConvert(r.Context(), r, req, h.opt)
	if err != nil {
		writeErrorFromErr(w, err)
		return
	}
	WriteText(w, http.StatusOK, out)
}
