package httpapi

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/John-Robertt/subconverter-go/internal/errlog"
)

type convertHandler struct {
	opt Options
}

func (h convertHandler) handleSub(w http.ResponseWriter, r *http.Request) {
	collector := errlog.NewCollector(ensureRequestID(r), r)

	req, err := parseConvertGET(r)
	if err != nil {
		writeErrorFromErr(w, r, err, collector, h.opt.ErrorLog)
		return
	}
	collector.SetRequest(req.Mode, string(req.Target), req.Subs, req.Profile, req.FileName, req.Encode)

	if err := setAttachmentHeaders(w, req); err != nil {
		writeErrorFromErr(w, r, err, collector, h.opt.ErrorLog)
		return
	}

	out, err := runConvert(r.Context(), r, req, h.opt, collector)
	if err != nil {
		writeErrorFromErr(w, r, err, collector, h.opt.ErrorLog)
		return
	}
	WriteText(w, http.StatusOK, out)
}

func (h convertHandler) handleConvert(w http.ResponseWriter, r *http.Request) {
	collector := errlog.NewCollector(ensureRequestID(r), r)

	// Prevent abusive payload sizes.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20 /* 1 MiB */)

	req, err := parseConvertPOST(r)
	if err != nil {
		writeErrorFromErr(w, r, err, collector, h.opt.ErrorLog)
		return
	}
	collector.SetRequest(req.Mode, string(req.Target), req.Subs, req.Profile, req.FileName, req.Encode)

	if err := setAttachmentHeaders(w, req); err != nil {
		writeErrorFromErr(w, r, err, collector, h.opt.ErrorLog)
		return
	}

	out, err := runConvert(r.Context(), r, req, h.opt, collector)
	if err != nil {
		writeErrorFromErr(w, r, err, collector, h.opt.ErrorLog)
		return
	}
	WriteText(w, http.StatusOK, out)
}

func (h convertHandler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if h.opt.ErrorLog != nil {
		status := h.opt.ErrorLog.Health(time.Now())
		if !status.Healthy {
			WriteText(w, http.StatusServiceUnavailable, "unhealthy: error-log-dir unavailable: "+status.Reason+"\n")
			return
		}
	}
	WriteText(w, http.StatusOK, "ok\n")
}

func (h convertHandler) handleErrorLogsZip(w http.ResponseWriter, r *http.Request) {
	if h.opt.ErrorLog == nil {
		writeErrorFromErr(w, r, apiError(http.StatusServiceUnavailable, newAppError("LOG_EXPORT_FAILED", "错误日志功能未启用", "export_logs"), nil), nil, nil)
		return
	}

	var buf bytes.Buffer
	if _, err := h.opt.ErrorLog.ExportZIP(&buf); err != nil {
		if errors.Is(err, errlog.ErrNoLogFiles) {
			writeErrorFromErr(w, r, apiError(http.StatusNotFound, newAppError("LOG_NOT_FOUND", "没有可下载的错误日志", "export_logs"), nil), nil, nil)
			return
		}
		log.Printf("export logs zip failed: %v", err)
		writeErrorFromErr(w, r, apiError(http.StatusInternalServerError, newAppError("LOG_EXPORT_FAILED", "导出错误日志失败", "export_logs"), err), nil, nil)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDispositionAttachment("subconverter-errors.zip"))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}
