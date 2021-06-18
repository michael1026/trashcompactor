package main

import (
	"bufio"
	"crypto/sha1"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/projectdiscovery/fastdialer/fastdialer"
)

type CookieInfo map[string]string

type SafeResources struct {
	mu        sync.Mutex
	resources map[string]bool
}

func (c *SafeResources) AddResource(key string) {
	c.mu.Lock()
	c.resources[key] = true
	c.mu.Unlock()
}

func (c *SafeResources) Value(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resources[key]
}

func buildHttpClient(jar cookiejar.Jar) (c *http.Client) {
	fastdialerOpts := fastdialer.DefaultOptions
	fastdialerOpts.EnableFallback = true
	dialer, err := fastdialer.NewDialer(fastdialerOpts)
	if err != nil {
		return
	}

	transport := &http.Transport{
		MaxIdleConns:      -1,
		IdleConnTimeout:   time.Second,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		DisableKeepAlives: true,
		DialContext:       dialer.Dial,
	}

	re := func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	client := &http.Client{
		Transport:     transport,
		CheckRedirect: re,
		Timeout:       time.Second * 3,
		Jar:           &jar,
	}

	return client
}

func main() {
	resources := SafeResources{resources: make(map[string]bool)}
	cookieFile := flag.String("C", "", "File containing cookie")

	flag.Parse()

	jar := readCookieJson(*cookieFile)

	client := buildHttpClient(*jar)

	wg := &sync.WaitGroup{}
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		wg.Add(1)
		time.Sleep(100 * time.Millisecond)
		go printUniqueContentURLs(s.Text(), client, wg, &resources)
	}

	wg.Wait()
}

func printUniqueContentURLs(url string, client *http.Client, wg *sync.WaitGroup, resources *SafeResources) {
	defer wg.Done()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Connection", "close")

	resp, err := client.Do(req)
	if err != nil {
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK && len(resp.Header.Get("content-type")) >= 9 && resp.Header.Get("content-type")[:9] == "text/html" {
		doc, err := goquery.NewDocumentFromReader(resp.Body)
		resource := ""
		h := sha1.New()

		if err != nil {
			return
		}

		doc.Find("script[src]").Each(func(index int, item *goquery.Selection) {
			src, _ := item.Attr("src")
			resource += src
		})

		doc.Find("script").Each(func(index int, item *goquery.Selection) {
			content, _ := item.Html()
			resource += fmt.Sprint(len(content))
		})

		h.Write([]byte(resource))
		bs := h.Sum(nil)

		if resources.Value(string(bs)) {
			return
		}

		resources.AddResource(string(bs))
		fmt.Println(url)
	}
}

func readCookiesFromString(s string) []*http.Cookie {
	cookieStrings := strings.Split(s, ";")

	for i, c := range cookieStrings {
		cookieStrings[i] = strings.TrimSpace(c)
	}

	cookieCount := len(cookieStrings)
	if cookieCount == 0 {
		return []*http.Cookie{}
	}
	cookies := make([]*http.Cookie, 0, cookieCount)
	for _, line := range cookieStrings {
		parts := strings.Split(strings.TrimSpace(line), ";")
		if len(parts) == 1 && parts[0] == "" {
			continue
		}
		parts[0] = strings.TrimSpace(parts[0])
		j := strings.Index(parts[0], "=")
		if j < 0 {
			continue
		}
		name, value := parts[0][:j], parts[0][j+1:]

		value, ok := parseCookieValue(value, true)
		if !ok {
			continue
		}
		c := &http.Cookie{
			Name:  name,
			Value: value,
			Raw:   line,
		}
		for i := 1; i < len(parts); i++ {
			parts[i] = strings.TrimSpace(parts[i])
			if len(parts[i]) == 0 {
				continue
			}

			attr, val := parts[i], ""
			if j := strings.Index(attr, "="); j >= 0 {
				attr, val = attr[:j], attr[j+1:]
			}
			lowerAttr := strings.ToLower(attr)
			val, ok = parseCookieValue(val, false)
			if !ok {
				c.Unparsed = append(c.Unparsed, parts[i])
				continue
			}
			switch lowerAttr {
			case "secure":
				c.Secure = true
				continue
			case "httponly":
				c.HttpOnly = true
				continue
			case "domain":
				c.Domain = val
				continue
			case "max-age":
				secs, err := strconv.Atoi(val)
				if err != nil || secs != 0 && val[0] == '0' {
					break
				}
				if secs <= 0 {
					secs = -1
				}
				c.MaxAge = secs
				continue
			case "expires":
				c.RawExpires = val
				exptime, err := time.Parse(time.RFC1123, val)
				if err != nil {
					exptime, err = time.Parse("Mon, 02-Jan-2006 15:04:05 MST", val)
					if err != nil {
						c.Expires = time.Time{}
						break
					}
				}
				c.Expires = exptime.UTC()
				continue
			case "path":
				c.Path = val
				continue
			}
			c.Unparsed = append(c.Unparsed, parts[i])
		}
		cookies = append(cookies, c)
	}
	return cookies
}

func parseCookieValue(raw string, allowDoubleQuote bool) (string, bool) {
	// Strip the quotes, if present.
	if allowDoubleQuote && len(raw) > 1 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	}
	return raw, true
}

func readCookieJson(filepath string) *cookiejar.Jar {
	jar, err := cookiejar.New(nil)

	if err != nil {
		log.Fatal("Error reading cookie file")
	}

	if filepath == "" {
		return jar
	}

	cookieFile, err := os.Open(filepath)
	var cookies CookieInfo

	if err != nil {
		log.Fatal("Error creating cookie jar")
	}

	defer cookieFile.Close()

	bytes, _ := ioutil.ReadAll(cookieFile)

	json.Unmarshal(bytes, &cookies)

	for rawUrl, cookieString := range cookies {
		parsedUrl, err := url.Parse(rawUrl)

		if err != nil {
			continue
		}

		jar.SetCookies(parsedUrl, readCookiesFromString(cookieString))
	}

	return jar
}
