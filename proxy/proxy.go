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
	"strings"
)

func main() {
	registry := flag.String("registry", "", "registry url to proxy requests")
	tracker := flag.String("tracker", "", "tracker url: http://host:port/")
	listen := flag.String("listen", "0.0.0.0:8888", "bind location")
	root := flag.String("root", "", "root dir to keep working files")
	flag.Parse()

	if *registry == "" || *tracker == "" || *root == "" {
		flag.PrintDefaults()
		return
	}

	// TODO: infer registry from request and create proxy on demand
	u, err := url.Parse(*registry)
	if err != nil {
		log.Fatal(err)
	}

	tu, err := url.Parse(*tracker)
	if err != nil {
		log.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(u)

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		log.Println("got request:", req.URL.Path)

		if !strings.HasPrefix(req.URL.Path, "/v1/images/") || !strings.HasSuffix(req.URL.Path, "/layer") {
			proxy.ServeHTTP(w, req)
			return
		}

		tu := *tu
		q := tu.Query()
		q.Set("url", "http://"+req.Host+req.URL.String())

		log.Println(req.URL.String())
		log.Printf("%#v", req.URL)
		log.Println(tu.String() + "?" + q.Encode())

		resp, err := http.Get(tu.String() + "?" + q.Encode())
		if err != nil {
			http.Error(w, "getting torrent failed", http.StatusInternalServerError)
			log.Println("getting torrent failed", err)
			return
		}

		defer resp.Body.Close()

		f, err := ioutil.TempFile(*root, "image-torrent-")
		if err != nil {
			http.Error(w, "torrent file creation failed", http.StatusInternalServerError)
			log.Println("torrent file creation failed", err)
			return
		}

		defer func() {
			f.Close()
			os.Remove(f.Name())
		}()

		_, err = io.Copy(f, resp.Body)
		if err != nil {
			http.Error(w, "reading torrent contents failed", http.StatusInternalServerError)
			log.Println("reading torrent contents failed", err)
			return
		}

		m, err := torrent.GetMetaInfo(nil, f.Name())
		if err != nil {
			http.Error(w, "reading torrent failed", http.StatusInternalServerError)
			log.Println("reading torrent failed", err)
			return
		}

		err = torrent.RunTorrents(&torrent.TorrentFlags{
			FileDir:   *root,
			SeedRatio: 0,
			//			UseDeadlockDetector: true,
		}, []string{f.Name()})

		// TODO: start another RunTorrents for configured interval
		// TODO: and remove data after that

		if err != nil {
			http.Error(w, "downloading torrent failed", http.StatusInternalServerError)
			log.Println("downloading torrent failed", err)
			return
		}

		l, err := os.Open(path.Join(*root, m.Info.Name))
		if err != nil {
			http.Error(w, "layer file open failed", http.StatusInternalServerError)
			log.Println("layer file open failed", err)
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
