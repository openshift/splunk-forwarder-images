package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const SplunkUser = "admin"
const SplunkHost = "127.0.0.1:8089"
const SplunkPath = "${SPLUNK_HOME}/bin/splunk"
const SplunkCACert = "${SPLUNK_HOME}/etc/auth/cacert.pem"
const UserSeedPath = "${SPLUNK_HOME}/etc/system/local/user-seed.conf"
const ServerConfigPath = "${SPLUNK_HOME}/etc/system/local/server.conf"
const HealthEndpoint = "/services/server/health/splunkd/details"
const SplunkPasswdPath = "${SPLUNK_HOME}/etc/passwd"
const SplunkdLogPath = "${SPLUNK_HOME}/var/log/splunk/splunkd.log"

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

const (
	splunkLicenseEnv        = "SPLUNK_LICENSE_ACCEPTED"
	splunkFlagAcceptLicense = "--accept-license"
)

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

func genPasswd() ([]byte, error) {
	os.Remove(os.ExpandEnv(SplunkPasswdPath))
	passwd := new(bytes.Buffer)
	if output, err := exec.Command(os.ExpandEnv(SplunkPath), "gen-random-passwd").Output(); err != nil {
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
	if seedFile, err := os.OpenFile(os.ExpandEnv(UserSeedPath), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644); err != nil {
		return err
	} else if passwd, err := genPasswd(); err != nil {
		return err
	} else {
		defer seedFile.Close()
		_, err = fmt.Fprintf(seedFile, "[user_info]\nUSERNAME = %s\nPASSWORD = %s\n", SplunkUser, string(passwd))
		return err
	}
}

func enableSplunkAPI() error {
	if serverFile, err := os.OpenFile(os.ExpandEnv(ServerConfigPath), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644); err != nil {
		return err
	} else {
		defer serverFile.Close()
		_, err = fmt.Fprintf(serverFile, `[sslConfig]
enableSplunkdSSL = false
[httpServer]
acceptFrom = 127.0.0.1/8
[proxyConfig]
http_proxy = %s
https_proxy = %s
no_proxy = %s
`, os.Getenv("HTTP_PROXY"), os.Getenv("HTTPS_PROXY"), os.Getenv("no_proxy"))

		return err
	}

}

var cmd *exec.Cmd
var ctx, _ = signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

func RunSplunk() bool {
	args := []string{"start", splunkFlagAcceptLicense, "--answer-yes", "--nodaemon"}
	args = append(args, os.Args[1:]...)
	cmd = exec.CommandContext(ctx, os.ExpandEnv(SplunkPath), args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	_ = cmd.Start()
	_ = cmd.Wait()
	return ctx.Err() == nil
}

func TailFile() bool {
	args := []string{"-F", os.ExpandEnv(SplunkdLogPath)}
	tail := exec.CommandContext(ctx, "/usr/bin/tail", args...)
	tail.Stdout = os.Stderr
	tail.Stderr = os.Stderr
	_ = tail.Start()
	_ = tail.Wait()
	return ctx.Err() == nil
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
			gauge := gaugeVec.WithLabelValues(k)
			if v.Healthy() {
				gauge.Set(0)
			} else {
				gauge.Set(1)
			}
		}
		handler.ServeHTTP(w, r)
	}))

	http.Handle("/livez", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("not ok"))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}
	}))

	http.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if health.Check() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("not ok"))
		}
		nok := map[bool]string{false: "not ok", true: "ok"}
		if r.URL.Query().Has("verbose") {
			for k, v := range health.Flatten() {
				w.Write([]byte("\n[+]" + k + " " + nok[v.Healthy()]))
			}
			w.Write([]byte("\n"))
		}
	}))

	http.ListenAndServe("0.0.0.0:8090", http.DefaultServeMux)
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

	if os.Getenv(splunkLicenseEnv) == "yes" {
		log.Println("splunk license agreement has been accepted")
	} else {
		log.Println("you must accept the terms of the Splunk licensing agreement before using this software.")
		log.Fatalf("set the variable %s to 'yes' to signal your acceptance of the licensing terms", splunkLicenseEnv)
	}

	go StartServer()

	go func() {
		for RunSplunk() {
			log.Println("splunkd exited, restarting in 5 seconds")
			time.Sleep(time.Second * 5)
		}
	}()

	for TailFile() {
		log.Println("tail exited, restarting in 5 seconds")
		time.Sleep(time.Second * 5)
	}
}
