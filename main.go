package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type Index struct {
	Versions map[string]Version `json:"versions"`
}

type Version struct {}

type Packages struct {
	Archives map[string]Archive `json:"archives"`
}

type Archive struct {
	URL string `json:"url"`
	Hashes []string `json:"hashes,omitempty"`
}

func uploadProvider(providersDir string) func(w http.ResponseWriter, r *http.Request) {
	if providersDir == "" {
		providersDir = "providers"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		providerBasePath := fmt.Sprintf("%s/%s/%s/%s", providersDir, vars["hostname"], vars["namespace"], vars["type"])

		r.ParseMultipartForm(32 << 20)  // max 32 MB
		file, handler, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Warn(err)
			return
		}
		defer file.Close()

		reg, _ := regexp.Compile(`^terraform-provider-([\w]+)_([\d.]+)_([\w_]+)`)  // terraform-provider-external_2.2.2_linux_amd64.zip
		providerName := reg.FindStringSubmatch(handler.Filename)[1]
		if providerName != vars["type"] {
			http.Error(w, "Provider name does not match upload path", http.StatusBadRequest)
			return
		}
		if filepath.Ext(handler.Filename) != ".zip" {
			http.Error(w, "Provider should be in ZIP archive", http.StatusUnsupportedMediaType)
		}
		providerVersion := reg.FindStringSubmatch(handler.Filename)[2]
		providerArch := reg.FindStringSubmatch(handler.Filename)[3]
		log.Debug(fmt.Sprintf("Uploading provider %s to %s, version %s, arch %s...", providerName, providerBasePath, providerVersion, providerArch))

		if _, err = os.Stat(providerBasePath); os.IsNotExist(err) {
			err = os.MkdirAll(providerBasePath, os.ModePerm)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				log.Warn(err)
				return
			}
		}

		var indexPrev Index
		var packagesPrev Packages
		versions := make(map[string]Version)
		archives := make(map[string]Archive)
		indexJson, err := os.ReadFile(fmt.Sprintf("%s/index.json", providerBasePath))
		if err == nil {
			json.Unmarshal(indexJson, &indexPrev)
			versions = indexPrev.Versions
		}
		packagesJson, err := os.ReadFile(fmt.Sprintf("%s/%s.json", providerBasePath, providerVersion))
		if err == nil {
			json.Unmarshal(packagesJson, &packagesPrev)
			archives = packagesPrev.Archives
		}
		
		versions[providerVersion] = Version{}
		index := Index{
			Versions: versions,
		}
		archives[providerArch] = Archive{
			URL: handler.Filename,
		}
		packages := Packages{
			Archives: archives,
		}
		
		b, _ := json.MarshalIndent(index, "", " ")
		err = os.WriteFile(fmt.Sprintf("%s/index.json", providerBasePath), b, 0644)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Warn(err)
			return
		}
		log.Debug(fmt.Sprintf("Updated %s/index.json", providerBasePath))
		b, _ = json.MarshalIndent(packages, "", " ")
		err = os.WriteFile(fmt.Sprintf("%s/%s.json", providerBasePath, providerVersion), b, 0644)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Warn(err)
			return
		}
		log.Debug(fmt.Sprintf("Updated %s/%s.json", providerBasePath, providerVersion))
		dst, err := os.Create(fmt.Sprintf("%s/%s", providerBasePath, handler.Filename))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Warn(err)
			return
		}
		defer dst.Close()
		_, err = io.Copy(dst, file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Warn(err)
			return
		}
		log.Debug("Provider uploaded")

		w.WriteHeader(http.StatusOK)
	}
}

func deleteProvider(providersDir string) func(w http.ResponseWriter, r *http.Request) {
	if providersDir == "" {
		providersDir = "providers"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		providerBasePath := fmt.Sprintf("%s/%s/%s/%s", providersDir, vars["hostname"], vars["namespace"], vars["type"])
		log.Debug(fmt.Sprintf("Deleting provider %s/%s/%s, version %s, arch %s...", vars["hostname"], vars["namespace"], vars["type"], vars["version"], vars["arch"]))

		// load {version}.json, get {path}.zip, delete version x arch, update/delete file
		var packages Packages
		var providerFilename string
		packagesJson, err := os.ReadFile(fmt.Sprintf("%s/%s.json", providerBasePath, vars["version"]))
		if err == nil {
			json.Unmarshal(packagesJson, &packages)
			if _, ok := packages.Archives[vars["arch"]]; ok {
				providerFilename = packages.Archives[vars["arch"]].URL
				delete(packages.Archives, vars["arch"])
				if len(packages.Archives) == 0 {
					if err = os.Remove(fmt.Sprintf("%s/%s.json", providerBasePath, vars["version"])); err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						log.Warn(err)
						return
					}
					log.Debug(fmt.Sprintf("Removed %s/%s.json", providerBasePath, vars["version"]))
				} else {
					b, _ := json.MarshalIndent(packages, "", " ")
					err = os.WriteFile(fmt.Sprintf("%s/%s.json", providerBasePath, vars["version"]), b, 0644)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						log.Warn(err)
						return
					}
					log.Debug(fmt.Sprintf("Updated %s/%s.json", providerBasePath, vars["version"]))
				}
			}
		}
		// load index.json, delete version if that was the last provider of that version, update/delete file
		if len(packages.Archives) == 0 {
			var index Index
			indexJson, err := os.ReadFile(fmt.Sprintf("%s/index.json", providerBasePath))
			if err == nil {
				json.Unmarshal(indexJson, &index)
				if _, ok := index.Versions[vars["version"]]; ok {
					delete(index.Versions, vars["version"])
					if len(index.Versions) == 0 {
						if err = os.Remove(fmt.Sprintf("%s/index.json", providerBasePath)); err != nil {
							http.Error(w, err.Error(), http.StatusInternalServerError)
							log.Warn(err)
							return
						}
						log.Debug(fmt.Sprintf("Removed %s/index.json", providerBasePath))
					} else {
						b, _ := json.MarshalIndent(index, "", " ")
						err = os.WriteFile(fmt.Sprintf("%s/index.json", providerBasePath), b, 0644)
						if err != nil {
							http.Error(w, err.Error(), http.StatusInternalServerError)
							log.Warn(err)
							return
						}
						log.Debug(fmt.Sprintf("Updated %s/index.json", providerBasePath))
					}
				}
			}
		}
		// check provider zips, if only one to delete is left then delete the whole directory, else only binary file
		providerArchives := make(map[string]bool)  // map for easier lookup
		files, err := ioutil.ReadDir(providerBasePath)
		if err == nil {
			for _, file := range files {
				if regexp.MustCompile(`^terraform-provider-[\w]+_[\d.]+_[\w_]+\.zip$`).MatchString(file.Name()) {
					providerArchives[file.Name()] = true
				}
			}
		}
		log.Debug(fmt.Sprintf("Provider archives: %v", providerArchives))
		if  _, ok := providerArchives[providerFilename]; ok {
			if len(providerArchives) == 1 {
				if err = os.RemoveAll(providerBasePath); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					log.Warn(err)
					return
				}
				log.Debug(fmt.Sprintf("Removed provider directory %s", providerBasePath))
			} else {
				if err = os.Remove(fmt.Sprintf("%s/%s", providerBasePath, providerFilename)); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					log.Warn(err)
					return
				}
				log.Debug(fmt.Sprintf("Removed provider %s/%s", providerBasePath, providerFilename))
			}
			log.Debug("Provider deleted")
		} else {
			http.Error(w, "Provider not found", http.StatusNotFound)
			log.Warn("Provider not found")
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        log.Debug(fmt.Sprintf("Received %s request to %s", r.Method, r.RequestURI))
        next.ServeHTTP(w, r)
    })
}

func main() {
	debug := flag.Bool("debug", false, "Debug mode")
	providersDir := flag.String("dir", "providers", "Directory to store providers in")
	production := flag.Bool("production", false, "Production mode which enables TLS and uses Let's Encrypt certificates")
	certFile := flag.String("cert", "cert.pem", "Path to cert file for TLS")
	keyFile := flag.String("key", "key.pem", "Path to key file for TLS")
	port := flag.Int("port", 8080, "Server port")
	flag.Parse()
	if *debug {
		log.SetLevel(log.DebugLevel)
		log.Debug("Log level set to debug")
	}

	fs := http.FileServer(http.Dir(*providersDir))
	router := mux.NewRouter()
	router.PathPrefix("/providers/").Methods("GET").Handler(http.StripPrefix("/providers/", fs))
	router.Path("/providers/{hostname}/{namespace}/{type}/upload/").Methods("POST").HandlerFunc(uploadProvider(*providersDir))
	router.Path("/providers/{hostname}/{namespace}/{type}/{version}/{arch}").Methods("DELETE").HandlerFunc(deleteProvider(*providersDir))
	router.Use(loggingMiddleware)

	srv := &http.Server{
		Handler: router,
		Addr: fmt.Sprintf("0.0.0.0:%d", *port),
		WriteTimeout: 15 * time.Second,
		ReadTimeout: 15 * time.Second,
		IdleTimeout:  time.Second * 60,
	}

	if *production {
		log.Info(fmt.Sprintf("Starting HTTPS server on 0.0.0.0:%d...", *port))
		go func() {
			if err := srv.ListenAndServeTLS(*certFile, *keyFile); err != nil {
				log.Warn(err)
			}
		}()

	} else {
		log.Info(fmt.Sprintf("Starting HTTP server on 0.0.0.0:%d...", *port))
		go func() {
			if err := srv.ListenAndServe(); err != nil {
				log.Warn(err)
			}
		}()
	}

	c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt) // quit via SIGINT (Ctrl+C)
    <-c
    ctx, cancel := context.WithTimeout(context.Background(), time.Second * 15)
    defer cancel()
    srv.Shutdown(ctx) // graceful shutdown
    log.Info("Shutting down...")
    os.Exit(0)
}