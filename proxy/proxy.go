package main

import (
	"flag"
	"github.com/jackpal/Taipei-Torrent/torrent"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
)

type DebugTransport struct{}

func (DebugTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	b, err := httputil.DumpRequestOut(r, false)
	if err != nil {
		return nil, err
	}
	log.Println(string(b))
	return http.DefaultTransport.RoundTrip(r)
}

func main() {
	tracker := flag.String("tracker", "", "tracker url: http://host:port/")
	listen := flag.String("listen", "0.0.0.0:8888", "bind location")
	root := flag.String("root", "", "root dir to keep working files")
	stripPort := flag.Bool("strip-port", false, "strip port from URL before proxying")
	tls := flag.Bool("tls", false, "upstream uses TLS")
	debug := flag.Bool("debug", false, "Log debug info")
	flag.Parse()

	if *tracker == "" || *root == "" {
		flag.PrintDefaults()
		return
	}

	tu, err := url.Parse(*tracker)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		u := *req.URL
		u.Host = strings.TrimPrefix(req.Host, "p2p-")

		if *stripPort {
			re := regexp.MustCompile(`^(.*):([0-9]*)$`)
			u.Host = re.ReplaceAllString(u.Host, `$1`)
		}

		if req.TLS != nil || *tls {
			u.Scheme = "https"
		} else {
			u.Scheme = "http"
		}

		log.Println("got request:", req.Method, u.String())

		v1r := regexp.MustCompile("/v1/images/.*/layer")
		v2r := regexp.MustCompile("/v2/.*/blobs/.*")

		if req.Method == "PUT" || !(v2r.MatchString(u.Path) || v1r.MatchString(u.Path)) {
			pu := u
			pu.Path = "/"
			proxy := httputil.NewSingleHostReverseProxy(&pu)

			if *debug {
				proxy.Transport = DebugTransport{}
			}

			// Update the headers to allow for SSL redirection
			req.Header.Set("X-Forwarded-Host", pu.Host)
			req.Header.Set("Host", pu.Host)
			req.Host = pu.Host

			proxy.ServeHTTP(w, req)
			return
		}

		tu := *tu
		q := tu.Query()
		q.Set("url", u.String())
		torrentURL := tu.String() + "?" + q.Encode()
		torrentClient := http.DefaultClient
		torrentReq, err := http.NewRequest("GET", torrentURL, nil)
		if err != nil {
			log.Fatalln(err)
		}

		authToken := req.Header.Get("Authorization")
		if authToken != "" {
			if *debug {
				log.Println("Found an authorization token: " + authToken)
			}
			torrentReq.Header.Set("Authorization", authToken)
		}

		log.Println(torrentURL)

		resp, err := torrentClient.Do(torrentReq)
		if err != nil {
			http.Error(w, "getting torrent failed", http.StatusInternalServerError)
			return
		}

		log.Printf("Status: %s\n", resp.Status)
		if resp.StatusCode != 200 {
			http.Error(w, "getting torrent failed", http.StatusInternalServerError)
			return
		}

		defer resp.Body.Close()

		f, err := ioutil.TempFile(*root, "image-torrent-")
		if err != nil {
			http.Error(w, "torrent file creation failed", http.StatusInternalServerError)
			log.Println("torrent file creation failed")
			return
		}

		defer func() {
			f.Close()
			os.Remove(f.Name())
		}()

		_, err = io.Copy(f, resp.Body)
		if err != nil {
			http.Error(w, "reading torrent contents failed", http.StatusInternalServerError)
			log.Println("reading torrent contents failed")
			return
		}

		m, err := torrent.GetMetaInfo(nil, f.Name())
		if err != nil {
			http.Error(w, "reading torrent failed", http.StatusInternalServerError)
			log.Println("reading torrent failed")
			return
		}

		err = torrent.RunTorrents(&torrent.TorrentFlags{
			FileDir:   *root,
			SeedRatio: 0,
		}, []string{f.Name()})

		lf := path.Join(*root, m.Info.Name)

		defer os.Remove(lf)

		// TODO: start another RunTorrents for configured interval
		// TODO: and remove data after that
		// TODO: or to hell with it

		if err != nil {
			http.Error(w, "downloading torrent failed", http.StatusInternalServerError)
			log.Println("downloading torrent failed")
			return
		}

		l, err := os.Open(lf)
		if err != nil {
			http.Error(w, "layer file open failed", http.StatusInternalServerError)
			log.Println("layer file open failed")
			return
		}

		defer l.Close()

		io.Copy(w, l)
	})

	err = http.ListenAndServe(*listen, mux)
	if err != nil {
		log.Fatal(err)
	}
}
