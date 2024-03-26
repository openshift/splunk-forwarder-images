package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const SplunkUser = "admin"
const SplunkHost = "127.0.0.1:8089"
const SplunkPath = "${SPLUNK_HOME}/bin/splunk"
const SplunkAppsCACert = "${SPLUNK_HOME}/etc/auth/appsCA.pem"
const SplunkCACert = "${SPLUNK_HOME}/etc/auth/ca.pem"
const SplunkTrustedBundle = "${SPLUNK_HOME}/etc/auth/ca-bundle.pem"
const TrustedCABundle = "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem"
const UserSeedPath = "${SPLUNK_HOME}/etc/system/local/user-seed.conf"
const ServerConfigPath = "${SPLUNK_HOME}/etc/system/local/server.conf"
const HealthEndpoint = "/services/server/health/splunkd/details"
const SplunkCryptPath = "${SPLUNK_HOME}/etc/passwd"
const SplunkServerConfTemplate = `
[httpServer]
acceptFrom = 127.0.0.0/8
[sslConfig]
enableSplunkdSSL = false
%s
`

type Status struct {
	Health  string
	Reasons *struct {
		Red struct {
			Primary struct {
				Indicator string
				Reason    string
			} `json:"1"`
		}
	} `json:"reasons,omitempty"`
}

type Feature struct {
	Status
	Features map[string]Feature `json:"features,omitempty"`
}

type SplunkHealth Feature

var healthURL = &url.URL{
	Scheme:   "http",
	Host:     SplunkHost,
	Path:     HealthEndpoint,
	RawQuery: url.Values{"output_mode": []string{"json"}}.Encode(),
}

func (s Feature) Flatten(prefix ...string) map[string]Status {
	out := map[string]Status{}
	for k, v := range s.Features {
		k = strings.ReplaceAll(k, " ", "")
		k = strings.ReplaceAll(k, "-", "")
		out[strings.Join(append(prefix, k), "/")] = v.Status
		for k2, v2 := range v.Flatten(append(prefix, k)...) {
			out[k2] = v2
		}
	}
	return out
}

func (s Status) Healthy() bool {
	return s.Health == "green"
}

func (s SplunkHealth) Flatten() map[string]Status {
	return (Feature)(s).Flatten()
}

func expandEnv(s string) string {
	if path, exists := os.LookupEnv("SPLUNK_HOME"); exists {
		if filepath.IsAbs(path) {
			if stat, err := os.Stat(path); err == nil && stat.IsDir() {
				return os.ExpandEnv(s)
			}
		}
	}
	log.Fatal("SPLUNK_HOME must be a directory")
	return s
}

func genPasswd() ([]byte, error) {
	os.Remove(os.ExpandEnv(SplunkCryptPath))
	passwd := new(bytes.Buffer)
	cmd := expandEnv(SplunkPath)
	args := []string{"gen-random-passwd"}
	if output, err := exec.Command(cmd, args...).Output(); err != nil {
		log.Fatal(err)
		return nil, err
	} else {
		passwd.Write(output[:8])
	}

	log.Println(passwd.String())
	healthURL.User = url.UserPassword(SplunkUser, passwd.String())
	return passwd.Bytes(), nil
}

func generateUserSeed() error {
	if seedFile, err := os.OpenFile(expandEnv(UserSeedPath), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644); err != nil {
		return err
	} else if passwd, err := genPasswd(); err != nil {
		return err
	} else {
		defer seedFile.Close()
		_, err = fmt.Fprintf(seedFile, "[user_info]\nUSERNAME = %s\nPASSWORD = %s\n", SplunkUser, string(passwd))
		return err
	}
}

func appendFile(dst io.Writer, path string) error {
	if src, err := os.OpenFile(path, os.O_RDONLY, 0644); err != nil {
		log.Print("Failed to open file: ", err)
		return err
	} else {
		defer src.Close()
		if _, err := io.Copy(dst, src); err != nil {
			log.Print("Failed to read file: ", err)
			return err
		}
	}
	_, err := dst.Write([]byte("\n"))
	return err
}

func enableSplunkAPI() error {

	if serverFile, err := os.OpenFile(os.ExpandEnv(ServerConfigPath), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644); err != nil {
		return err
	} else {
		defer serverFile.Close()

		if err := exec.Command("/usr/bin/update-ca-trust").Run(); err != nil {
			log.Fatal(err)
			return err
		}

		if caFile, err := os.OpenFile(os.ExpandEnv(SplunkTrustedBundle), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644); err != nil {
			log.Fatal(err)
			return err
		} else {
			defer caFile.Close()
			if err := appendFile(caFile, TrustedCABundle); err != nil {
				return err
			}
			if err := appendFile(caFile, os.ExpandEnv(SplunkAppsCACert)); err != nil {
				return err
			}
		}

		proxyConfig := ""

		if http_proxy, exists := os.LookupEnv("HTTP_PROXY"); exists {
			proxyConfig += "http_proxy = " + http_proxy + "\n"
		}

		if https_proxy, exists := os.LookupEnv("HTTPS_PROXY"); exists {
			proxyConfig += "https_proxy = " + https_proxy + "\n"
		}

		if len(proxyConfig) != 0 {

			if no_proxy, exists := os.LookupEnv("NO_PROXY"); exists {
				proxyConfig += "no_proxy = " + no_proxy + "\n"
			}

			proxyConfig = "sslRootCAPath = " + expandEnv(SplunkTrustedBundle) + "\n[proxyConfig]\n" + proxyConfig
			proxyConfig += "proxy_rules = splunkcloud.com\n"

		}

		_, err = fmt.Fprintf(serverFile, SplunkServerConfTemplate, proxyConfig)

		return err
	}

}

var cmd *exec.Cmd

var ctx, _ = signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

func RunSplunk() bool {
	name := expandEnv(SplunkPath)
	args := []string{"start", "--answer-yes", "--nodaemon"}
	args = append(args, os.Args[1:]...)
	cmd = exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if cmd.Start() != nil {
		return false
	}
	if cmd.Wait() != nil {
		return false
	}
	return ctx.Err() == nil
}

func writeResponse(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	if _, err := w.Write([]byte(message + "\n")); err != nil {
		log.Print("failed writing response: ", err.Error())
	}
}

func StartServer() {

	var health = &SplunkHealth{}

	var gaugeVec = *prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "splunk_forwarder",
		Subsystem: "component",
		Name:      "unhealthy",
	}, []string{"component"})

	reg := prometheus.NewRegistry()
	reg.MustRegister(gaugeVec)

	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		Registry: reg,
	})

	http.Handle("/metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if health.Check() {
			gaugeVec.Reset()
		}
		for k, v := range health.Flatten() {
			guage := gaugeVec.WithLabelValues(k)
			if v.Healthy() {
				guage.Set(0)
			} else {
				guage.Set(1)
			}
		}
		handler.ServeHTTP(w, r)
	}))

	http.Handle("/livez", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			writeResponse(w, http.StatusInternalServerError, "not ok")
		} else {
			writeResponse(w, http.StatusOK, "ok")
		}
	}))

	http.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if health.Check() {
			writeResponse(w, http.StatusOK, "ok")
		} else {
			writeResponse(w, http.StatusInternalServerError, "not ok")
		}
		nok := map[bool]string{false: "not ok", true: "ok"}
		if r.URL.Query().Has("verbose") {
			for k, v := range health.Flatten() {
				if _, err := w.Write([]byte("[+]" + k + " " + nok[v.Healthy()] + "\n")); err != nil {
					log.Print("failed writing response: ", err.Error())
					break
				}
			}
		}
	}))

	l, err := net.Listen("tcp", ":8090")
	if err != nil { // nolint
		log.Fatal(err)
	}
	s := http.Server{Handler: http.DefaultServeMux, ReadHeaderTimeout: time.Second * 5}

	log.Fatal(s.Serve(l))

}

func (h *SplunkHealth) Check() bool {
	res, err := http.Get(healthURL.String())
	if err != nil {
		log.Println("health endpoint request failed: ", err.Error())
		return false
	}
	obj := struct {
		Entry []struct{ Content *SplunkHealth }
	}{}
	if err := json.NewDecoder(res.Body).Decode(&obj); err != nil {
		log.Println("failed parsing health endpoint response: ", err.Error())
		return false
	}
	for i := range obj.Entry {
		if obj.Entry[i].Content != nil {
			*h = *(obj.Entry[i].Content)
			return h.Healthy()
		}
	}
	return false
}

func main() {

	if err := generateUserSeed(); err != nil {
		log.Fatal("couldn't generate admin user seed: ", err.Error())
	}

	if err := enableSplunkAPI(); err != nil {
		log.Fatal("couldn't enable splunk api: ", err.Error())
	}

	go StartServer()

	for RunSplunk() {
		log.Println("splunkd exited, restarting in 5 seconds")
		time.Sleep(time.Second * 5)
	}

}
