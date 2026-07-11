package scrape

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"image"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gocolly/colly/v2"
	"github.com/mozillazg/go-slugify"
	"github.com/thoas/go-funk"
	"github.com/tidwall/gjson"
	"github.com/xbapps/xbvr/pkg/models"
	_ "golang.org/x/image/webp"
)

type vrpSitemapIndex struct {
	XMLName  xml.Name     `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 sitemapindex"`
	Sitemaps []vrpSitemap `xml:"sitemap"`
}

type vrpSitemap struct {
	Loc string `xml:"loc"`
}

type vrpURLSet struct {
	XMLName xml.Name `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 urlset"`
	URLs    []vrpURL `xml:"url"`
}

type vrpURL struct {
	Loc   string   `xml:"loc"`
	Image vrpImage `xml:"http://www.google.com/schemas/sitemap-image/1.1 image"`
}

type vrpImage struct {
	Loc string `xml:"http://www.google.com/schemas/sitemap-image/1.1 loc"`
}

type vrpVideoLD struct {
	Type        string `json:"@type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	UploadDate  string `json:"uploadDate"`
	Duration    string `json:"duration"`
}

func parseISODurationMinutes(s string) int {
	re := regexp.MustCompile(`PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	total := 0
	if m[1] != "" {
		if h, err := strconv.Atoi(m[1]); err == nil {
			total += h * 60
		}
	}
	if m[2] != "" {
		if mn, err := strconv.Atoi(m[2]); err == nil {
			total += mn
		}
	}
	if m[3] != "" {
		if sec, err := strconv.Atoi(m[3]); err == nil && sec > 0 {
			total++
		}
	}
	return total
}

func fetchVRPXML(u string) ([]byte, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Cookie", "vrn_age_gate=1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func VirtualRealPornSite(wg *models.ScrapeWG, updateSite bool, knownScenes []string, out chan<- models.ScrapedScene, singleSceneURL string, scraperID string, siteID string, URL string, singeScrapeAdditionalInfo string, limitScraping bool) error {
	defer wg.Done()
	logScrapeStart(scraperID, siteID)
	imageCollector := createCollector("virtualrealporn.com", "virtualrealtrans.com", "virtualrealgay.com", "virtualrealpassion.com", "virtualrealamateurporn.com", "static.virtualrealhub.com")
	sceneCollector := createCollector("virtualrealporn.com", "virtualrealtrans.com", "virtualrealgay.com", "virtualrealpassion.com", "virtualrealamateurporn.com")

	// Bypass age-gate overlay on all requests
	for _, c := range []*colly.Collector{imageCollector, sceneCollector} {
		c.OnRequest(func(r *colly.Request) {
			r.Headers.Set("Cookie", "vrn_age_gate=1")
		})
	}

	imageCollector.OnResponse(func(r *colly.Response) {
		if _, _, err := image.Decode(bytes.NewReader(r.Body)); err == nil {
			r.Ctx.Put("valid", "1")
		}
	})

	sceneCollector.OnHTML(`html`, func(e *colly.HTMLElement) {
		sc := models.ScrapedScene{}
		sc.ScraperID = scraperID
		sc.SceneType = "VR"
		sc.Studio = "VirtualRealPorn"
		sc.Site = siteID
		sc.HomepageURL = strings.Split(e.Request.URL.String(), "?")[0]

		// Scene ID from sitemap context or from cover image URL
		sc.SiteID = e.Request.Ctx.Get("siteID")
		if sc.SiteID == "" {
			if m := regexp.MustCompile(`/videos/(\d+)/`).FindStringSubmatch(e.ChildAttr(`picture.vdi-cover__poster source`, "srcset")); m != nil {
				sc.SiteID = m[1]
			} else if m := regexp.MustCompile(`/videos/(\d+)/`).FindStringSubmatch(e.ChildAttr(`picture.vdi-cover__poster img`, "src")); m != nil {
				sc.SiteID = m[1]
			}
		}
		if sc.SiteID != "" {
			sc.SceneID = slugify.Slugify(sc.Site) + "-" + sc.SiteID
		}

		// JSON-LD VideoObject (title, description, date, duration)
		var videoObj vrpVideoLD
		e.ForEach(`script[type="application/ld+json"]`, func(_ int, el *colly.HTMLElement) {
			if videoObj.Type != "" {
				return
			}
			var v vrpVideoLD
			if err := json.Unmarshal([]byte(el.Text), &v); err == nil && v.Type == "VideoObject" {
				videoObj = v
			}
		})

		// Title
		if videoObj.Name != "" {
			sc.Title = html.UnescapeString(videoObj.Name)
		} else {
			e.ForEach(`title`, func(id int, e *colly.HTMLElement) {
				sc.Title = strings.TrimSpace(strings.Split(e.Text, "|")[0])
				sc.Title = strings.TrimSpace(strings.Replace(sc.Title, "▷ ", "", -1))
				sc.Title = strings.TrimSpace(strings.Replace(sc.Title, fmt.Sprintf(" - %v.com", sc.Site), "", -1))
			})
		}

		// Synopsis
		if videoObj.Description != "" {
			sc.Synopsis = html.UnescapeString(videoObj.Description)
		}

		// Release date and duration
		if videoObj.UploadDate != "" {
			sc.Released = strings.Split(videoObj.UploadDate, "T")[0]
		}
		if videoObj.Duration != "" {
			sc.Duration = parseISODurationMinutes(videoObj.Duration)
		}

		// Tags
		e.ForEach(`.vd-tags__tag`, func(id int, e *colly.HTMLElement) {
			sc.Tags = append(sc.Tags, strings.TrimSpace(e.Text))
		})
		if scraperID == "virtualrealgay" {
			sc.Tags = append(sc.Tags, "Gay")
		}

		// Cast
		sc.ActorDetails = make(map[string]models.ActorDetails)
		e.ForEach(`.vd-pornstar__link`, func(_ int, el *colly.HTMLElement) {
			name := strings.TrimSpace(el.ChildText(".vd-pornstar__name"))
			if name == "" {
				return
			}
			name = strings.TrimSuffix(name, " VR")
			sc.Cast = append(sc.Cast, name)
			sc.ActorDetails[name] = models.ActorDetails{Source: scraperID + " scrape", ProfileUrl: strings.SplitN(e.Request.AbsoluteURL(el.Attr("href")), "?", 2)[0]}
		})

		// Cover URLs (prefer webp poster, fallback to jpg)
		e.ForEach(`picture.vdi-cover__poster source[type="image/webp"]`, func(id int, e *colly.HTMLElement) {
			if len(sc.Covers) == 0 {
				u := strings.Split(e.Request.AbsoluteURL(e.Attr("srcset")), "?")[0]
				ctx := colly.NewContext()
				if err := imageCollector.Request("GET", u, nil, ctx, nil); err == nil {
					if ctx.Get("valid") != "" {
						sc.Covers = append(sc.Covers, u)
					}
				}
			}
		})
		e.ForEach(`picture.vdi-cover__poster img`, func(id int, e *colly.HTMLElement) {
			if len(sc.Covers) == 0 {
				u := strings.Split(e.Request.AbsoluteURL(e.Attr("src")), "?")[0]
				ctx := colly.NewContext()
				if err := imageCollector.Request("GET", u, nil, ctx, nil); err == nil {
					if ctx.Get("valid") != "" {
						sc.Covers = append(sc.Covers, u)
					}
				}
			}
		})

		// Gallery
		e.ForEach(`a.vd-screenshots__item[data-gallery-src]`, func(id int, e *colly.HTMLElement) {
			u := e.Request.AbsoluteURL(strings.Split(e.Attr("data-gallery-src"), "?")[0])
			if len(sc.Covers) == 0 {
				sc.Covers = append(sc.Covers, u)
			} else {
				sc.Gallery = append(sc.Gallery, u)
			}
		})

		// Filenames (old download-links script still present on some pages)
		e.ForEach(`script[id="virtualreal_download-links-js-extra"]`, func(id int, e *colly.HTMLElement) {
			if id == 0 {
				jsonData := e.Text[strings.Index(e.Text, "{") : len(e.Text)-12]
				fpName := gjson.Get(jsonData, "videopart").String()

				if fpName == "" {
					return
				}

				siteIDAcronym := "VRP"
				if siteID == "VirtualRealTrans" {
					siteIDAcronym = "VRT"
				}
				if siteID == "VirtualRealAmateurPorn" {
					siteIDAcronym = "VRAM"
				}
				if siteID == "VirtualRealGay" {
					siteIDAcronym = "VRG"
				}
				if siteID == "VirtualRealPassion" {
					siteIDAcronym = "VRPA"
				}

				var outFilenames []string

				// Playstation VR
				outFilenames = append(outFilenames, siteIDAcronym+"_"+fpName+"_Trailer_PS4_180_sbs.mp4") // Trailer
				outFilenames = append(outFilenames, siteID+"_"+fpName+"_3K_180_sbs.mp4")                 // PS4
				outFilenames = append(outFilenames, siteIDAcronym+"_"+fpName+"_180_sbs.mp4")             // PS4 (older videos)
				outFilenames = append(outFilenames, siteID+".com_-_"+fpName+"_-_180_sbs.mp4")            // PS4 (oldest videos)
				outFilenames = append(outFilenames, siteID+"_"+fpName+"_4096x2040_180_sbs.mp4")          // PS4 Pro
				outFilenames = append(outFilenames, siteIDAcronym+"_"+fpName+"_Pro_180_sbs.mp4")         // PS4 Pro (older videos)
				outFilenames = append(outFilenames, siteID+".com_-_"+fpName+"_-_Pro_180_sbs.mp4")        // PS4 Pro (oldest videos)

				// Oculus Go / Quest
				outFilenames = append(outFilenames, siteID+".com_-_"+fpName+"_-_Trailer.mp4")       // Trailer (same for Oculus Rift (S) / Vive / Windows MR)
				outFilenames = append(outFilenames, siteID+"_"+fpName+"_4864_180x180_3dh.mp4")      // 4K+
				outFilenames = append(outFilenames, siteID+"_-_"+fpName+"_-_h264P_180x180_3dh.mp4") // 4K+ (older videos)
				outFilenames = append(outFilenames, siteID+"_"+fpName+"_4K_30M_180x180_3dh.mp4")    // 4K HQ (same for Gear VR / Daydream and Oculus Rift (S) / Vive / Windows MR)
				outFilenames = append(outFilenames, siteID+"_"+fpName+"_4K_h265_180x180_3dh.mp4")   // 4K h265 (same for Oculus Rift (S) / Vive / Windows MR)
				outFilenames = append(outFilenames, siteID+"_-_"+fpName+"_-_vp9_180x180_3dh.mp4")   // 4K VP9 (older videos; same for Gear VR / Daydream and Oculus Rift (S) / Vive / Windows MR)
				outFilenames = append(outFilenames, siteID+"_-_"+fpName+"_-_180x180_3dh.mp4")       // 4K h264 (older videos; same for Gear VR / Daydream)

				// Gear VR / Daydream
				outFilenames = append(outFilenames, siteID+"_-_"+fpName+"_-_Trailer_Streaming_3dh.mp4") // Trailer
				outFilenames = append(outFilenames, siteID+"_"+fpName+"_4K_180x180_3dh.mp4")            // 4K (same for Smartphone)

				// Smartphone
				outFilenames = append(outFilenames, siteID+".com_-_"+fpName+"_-_Trailer_-_Smartphone.mp4") // Trailer
				outFilenames = append(outFilenames, siteID+"_"+fpName+"_1920_180x180_3dh.mp4")             // Full HD (same for Oculus Rift (S) / Vive / Windows MR)
				outFilenames = append(outFilenames, siteID+".com_-_"+fpName+"_-_1920.mp4")                 // Full HD (older videos; same for Oculus Rift (S) / Vive / Windows MR)

				// Oculus Rift (S) / Vive / Windows MR
				outFilenames = append(outFilenames, siteID+"_"+fpName+"_8K_180x180_3dh.mp4")         // 5K
				outFilenames = append(outFilenames, siteID+"_"+fpName+"_5K_30M_180x180_3dh.mp4")     // 5K HQ
				outFilenames = append(outFilenames, siteID+"_"+fpName+"_5K_180x180_3dh.mp4")         // 5K
				outFilenames = append(outFilenames, siteID+"_-_"+fpName+"_-_5K_180x180_3dh.mp4")     // 5K (older videos)
				outFilenames = append(outFilenames, siteID+".com_-_"+fpName+"_-_5K_180x180_3dh.mp4") // 5K (before site update)
				outFilenames = append(outFilenames, siteID+".com_-_"+fpName+"_-_h264.mp4")           // 4K 264 (older videos)

				sc.Filenames = outFilenames
			}
		})

		// Trailer
		slug := ""
		parts := strings.Split(strings.Trim(sc.HomepageURL, "/"), "/")
		if len(parts) > 0 {
			slug = parts[len(parts)-1]
		}
		if slug != "" {
			sc.TrailerType = "url"
			sc.TrailerSrc = e.Request.AbsoluteURL("/videos/" + slug + "/download/oculus/")
		}

		if sc.SceneID != "" && sc.Title != "" {
			out <- sc
		}
	})

	// Fetch sitemap index to locate the videos sitemap for this domain
	var videoSitemapURL string
	if indexBytes, err := fetchVRPXML(URL + "sitemap.xml"); err == nil {
		var sidx vrpSitemapIndex
		if err := xml.Unmarshal(indexBytes, &sidx); err == nil {
			for _, sm := range sidx.Sitemaps {
				if strings.Contains(sm.Loc, "videos_sitemap.xml") {
					videoSitemapURL = sm.Loc
					break
				}
			}
		}
	}
	if videoSitemapURL == "" {
		videoSitemapURL = URL + "sitemaps/" + GetCoreDomain(URL) + "/videos_sitemap.xml"
	}

	sitemapBytes, err := fetchVRPXML(videoSitemapURL)
	if err != nil {
		log.Errorf("Error fetching VRP sitemap %s: %v", videoSitemapURL, err)
		logScrapeFinished(scraperID, siteID)
		return nil
	}

	var uset vrpURLSet
	if err := xml.Unmarshal(sitemapBytes, &uset); err != nil {
		log.Errorf("Error parsing VRP sitemap %s: %v", videoSitemapURL, err)
		logScrapeFinished(scraperID, siteID)
		return nil
	}

	siteIDRegex := regexp.MustCompile(`/videos/(\d+)/`)
	sceneCount := 0
	for _, u := range uset.URLs {
		sceneURL := strings.Split(u.Loc, "?")[0]

		if funk.ContainsString(knownScenes, sceneURL) {
			continue
		}

		id := ""
		if u.Image.Loc != "" {
			if m := siteIDRegex.FindStringSubmatch(u.Image.Loc); m != nil {
				id = m[1]
			}
		}

		ctx := colly.NewContext()
		ctx.Put("siteID", id)
		sceneCollector.Request("GET", sceneURL, nil, ctx, nil)

		sceneCount++
		if limitScraping && sceneCount >= 20 {
			break
		}
	}

	if singleSceneURL != "" {
		sceneCollector.Visit(singleSceneURL)
	}

	if updateSite {
		updateSiteLastUpdate(scraperID)
	}
	logScrapeFinished(scraperID, siteID)
	return nil
}

func VirtualRealPorn(wg *models.ScrapeWG, updateSite bool, knownScenes []string, out chan<- models.ScrapedScene, singleSceneURL string, singeScrapeAdditionalInfo string, limitScraping bool) error {
	return VirtualRealPornSite(wg, updateSite, knownScenes, out, singleSceneURL, "virtualrealporn", "VirtualRealPorn", "https://virtualrealporn.com/", singeScrapeAdditionalInfo, limitScraping)
}
func VirtualRealTrans(wg *models.ScrapeWG, updateSite bool, knownScenes []string, out chan<- models.ScrapedScene, singleSceneURL string, singeScrapeAdditionalInfo string, limitScraping bool) error {
	return VirtualRealPornSite(wg, updateSite, knownScenes, out, singleSceneURL, "virtualrealtrans", "VirtualRealTrans", "https://virtualrealtrans.com/", singeScrapeAdditionalInfo, limitScraping)
}
func VirtualRealAmateur(wg *models.ScrapeWG, updateSite bool, knownScenes []string, out chan<- models.ScrapedScene, singleSceneURL string, singeScrapeAdditionalInfo string, limitScraping bool) error {
	return VirtualRealPornSite(wg, updateSite, knownScenes, out, singleSceneURL, "virtualrealamateur", "VirtualRealAmateurPorn", "https://virtualrealamateurporn.com/", singeScrapeAdditionalInfo, limitScraping)
}
func VirtualRealGay(wg *models.ScrapeWG, updateSite bool, knownScenes []string, out chan<- models.ScrapedScene, singleSceneURL string, singeScrapeAdditionalInfo string, limitScraping bool) error {
	return VirtualRealPornSite(wg, updateSite, knownScenes, out, singleSceneURL, "virtualrealgay", "VirtualRealGay", "https://virtualrealgay.com/", singeScrapeAdditionalInfo, limitScraping)
}
func VirtualRealPassion(wg *models.ScrapeWG, updateSite bool, knownScenes []string, out chan<- models.ScrapedScene, singleSceneURL string, singeScrapeAdditionalInfo string, limitScraping bool) error {
	return VirtualRealPornSite(wg, updateSite, knownScenes, out, singleSceneURL, "virtualrealpassion", "VirtualRealPassion", "https://virtualrealpassion.com/", singeScrapeAdditionalInfo, limitScraping)
}

func init() {
	registerScraper("virtualrealporn", "VirtualRealPorn", "https://pbs.twimg.com/profile_images/921297545195859968/E5-ClWkm_200x200.jpg", "virtualrealporn.com", VirtualRealPorn)
	registerScraper("virtualrealtrans", "VirtualRealTrans", "https://pbs.twimg.com/profile_images/921298616970555392/3coTQ6UZ_200x200.jpg", "virtualrealtrans.com", VirtualRealTrans)
	registerScraper("virtualrealgay", "VirtualRealGay", "https://pbs.twimg.com/profile_images/921298132129992704/jIOE0LxX_200x200.jpg", "virtualrealgay.com", VirtualRealGay)
	registerScraper("virtualrealpassion", "VirtualRealPassion", "https://pbs.twimg.com/profile_images/921298874249175041/LjWabMPh_200x200.jpg", "virtualrealpassion.com", VirtualRealPassion)
	registerScraper("virtualrealamateur", "VirtualRealAmateurPorn", "https://mcdn.vrporn.com/files/20170718094205/virtualrealameteur-vr-porn-studio-vrporn.com-virtual-reality.png", "virtualrealamateurporn.com", VirtualRealAmateur)
}
