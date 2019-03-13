// Copyright © 2018 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/ory/go-convenience/stringsx"
	"github.com/ory/graceful"
	"github.com/patrickmn/go-cache"
	"github.com/rs/cors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type roundTripper func(r *http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func id(r *http.Request) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s", r.Method, r.URL.String())))
	return string(h[:])
}

type cacheItem struct {
	Status           string // e.g. "200 OK"
	StatusCode       int    // e.g. 200
	Proto            string // e.g. "HTTP/1.0"
	ProtoMajor       int    // e.g. 1
	ProtoMinor       int    // e.g. 0
	Header           http.Header
	Body             []byte
	ContentLength    int64
	TransferEncoding []string
	Close            bool
	Uncompressed     bool
	Trailer          http.Header
}

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		log := logrus.New()
		c := cache.New(5*time.Minute, 10*time.Minute)

		h := &httputil.ReverseProxy{
			Director: func(r *http.Request) {
				r.URL.Host = r.URL.Query().Get("__host")
				r.URL.Scheme = stringsx.Coalesce(r.URL.Query().Get("__proto"), "https")
				r.Host = r.URL.Host
				q := r.URL.Query()
				q.Del("__host")
				q.Del("__proto")
				r.Header.Del("Cookie")
				r.URL.RawQuery = q.Encode()
				if _, ok := r.Header["User-Agent"]; !ok {
					r.Header.Set("User-Agent", "")
				}

				r.Header.Del("If-Modified-Since")
				r.Header.Del("If-None-Match")
				r.Header.Del("Cache-Control")
			},
			Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
				if i, found := c.Get(id(r)); found {
					var ci cacheItem
					err := gob.NewDecoder(bytes.NewBuffer(i.([]byte))).Decode(&ci)
					header := ci.Header
					header.Set("X-Cache", "hit")
					return &http.Response{
						Status:           ci.Status,
						Proto:            ci.Proto,
						ProtoMajor:       ci.ProtoMajor,
						ProtoMinor:       ci.ProtoMinor,
						Header:           ci.Header,
						ContentLength:    ci.ContentLength,
						TransferEncoding: ci.TransferEncoding,
						Close:            ci.Close,
						Trailer:          ci.Trailer,
						Uncompressed:     ci.Uncompressed,
						StatusCode:       ci.StatusCode,
						Body:             ioutil.NopCloser(bytes.NewBuffer(ci.Body)),
					}, err
				}

				res, err := http.DefaultTransport.RoundTrip(r)
				if err != nil {
					return res, err
				}

				body, err := ioutil.ReadAll(res.Body)
				if err != nil {
					return res, err
				}

				if err := res.Body.Close(); err != nil {
					return res, err
				}

				res.Header.Del("Access-Control-Allow-Origin")
				res.Header.Del("Access-Control-Allow-Methods")
				res.Header.Del("Access-Control-Allow-Headers")
				res.Header.Del("Access-Control-Max-Age")

				var b bytes.Buffer
				if err := gob.NewEncoder(&b).Encode(&cacheItem{
					Status:           res.Status,
					Proto:            res.Proto,
					ProtoMajor:       res.ProtoMajor,
					ProtoMinor:       res.ProtoMinor,
					Header:           res.Header,
					ContentLength:    res.ContentLength,
					TransferEncoding: res.TransferEncoding,
					Close:            res.Close,
					Trailer:          res.Trailer,
					Uncompressed:     res.Uncompressed,
					StatusCode:       res.StatusCode,
					Body:             body,
				}); err != nil {
					return res, err
				}

				if len(body) > 0 {
					c.SetDefault(id(r), b.Bytes())
				}

				res.Header.Set("X-Cache", "miss")
				res.Body = ioutil.NopCloser(bytes.NewBuffer(body))
				return res, err
			}),
		}

		xo := cors.New(cors.Options{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{
				http.MethodConnect, http.MethodPost, http.MethodDelete, http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPatch, http.MethodPut, http.MethodTrace,
			},
			AllowedHeaders:   []string{},
			ExposedHeaders:   []string{},
			AllowCredentials: false,
			Debug:            false,
		})
		server := graceful.WithDefaults(&http.Server{
			Addr:    fmt.Sprintf("%s:%s", viper.GetString("HOST"), viper.GetString("PORT")),
			Handler: xo.Handler(h),
		})

		log.Printf("main: Starting the server at: %s", server.Addr)
		if err := graceful.Graceful(server.ListenAndServe, server.Shutdown); err != nil {
			log.Fatalln("main: Failed to gracefully shutdown")
		}
		log.Println("main: Server was shutdown gracefully")
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// serveCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// serveCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
