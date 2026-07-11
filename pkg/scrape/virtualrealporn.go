package scrape

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"image"
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

func parseVRPMetaDuration(s string) int {
	s = strings.ToUpper(strings.TrimSpace(s))
	// Format examples: "13:20 MIN | ...", "1:05:30 MIN | ..."
	if m := regexp.MustCompile(`(\d+):(\d+):(\d+)\s*MIN`).FindStringSubmatch(s); m != nil {
		h, _ := strconv.Atoi(m[1])
		mn, _ := strconv.Atoi(m[2])
		return h*60 + mn
	}
	if m := regexp.MustCompile(`(\d+):(\d+)\s*MIN`).FindStringSubmatch(s); m != nil {
		mn, _ := strconv.Atoi(m[1])
		return mn
	}
	return 0
}

func VirtualRealPornSite(wg *models.ScrapeWG, updateSite bool, knownScenes []string, out chan<- models.ScrapedScene, singleSceneURL string, scraperID string, siteID string, URL string, singeScrapeAdditionalInfo string, limitScraping bool) error {
	defer wg.Done()
	logScrapeStart(scraperID, siteID)
	imageCollector := createCollector("virtualrealporn.com", "virtualrealtrans.com", "virtualrealgay.com", "virtualrealpassion.com", "virtualrealamateurporn.com", "static.virtualrealhub.com")
	sceneCollector := createCollector("virtualrealporn.com", "virtualrealtrans.com", "virtualrealgay.com", "virtualrealpassion.com", "virtualrealamateurporn.com")
	siteCollector := createCollector("virtualrealporn.com", "virtualrealtrans.com", "virtualrealgay.com", "virtualrealpassion.com", "virtualrealamateurporn.com")

	// Bypass age-gate overlay on all requests
	for _, c := range []*colly.Collector{imageCollector, sceneCollector, siteCollector} {
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

		// Scene ID passed from list card thumbnail URL
		sc.SiteID = e.Request.Ctx.Get("siteID")
		if sc.SiteID != "" {
			sc.SceneID = slugify.Slugify(sc.Site) + "-" + sc.SiteID
		}

		// JSON-LD fallback (date, title, description, duration)
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
		if t := strings.TrimSpace(e.ChildText(".vdi-info__title")); t != "" {
			sc.Title = html.UnescapeString(t)
		} else if videoObj.Name != "" {
			sc.Title = html.UnescapeString(videoObj.Name)
		} else {
			e.ForEach(`title`, func(id int, e *colly.HTMLElement) {
				sc.Title = strings.TrimSpace(strings.Split(e.Text, "|")[0])
				sc.Title = strings.TrimSpace(strings.Replace(sc.Title, "▷ ", "", -1))
				sc.Title = strings.TrimSpace(strings.Replace(sc.Title, fmt.Sprintf(" - %v.com", sc.Site), "", -1))
			})
		}

		// Synopsis / description
		if d := strings.TrimSpace(e.ChildText(".vdi-info__description-text")); d != "" {
			sc.Synopsis = html.UnescapeString(d)
		} else if videoObj.Description != "" {
			sc.Synopsis = html.UnescapeString(videoObj.Description)
		}

		// Duration from meta text (floor to minutes), fallback to JSON-LD
		if metaText := strings.TrimSpace(e.ChildText(".vdi-info__meta-text")); metaText != "" {
			sc.Duration = parseVRPMetaDuration(metaText)
		}
		if sc.Duration == 0 && videoObj.Duration != "" {
			sc.Duration = parseISODurationMinutes(videoObj.Duration)
		}

		// Release date from JSON-LD
		if videoObj.UploadDate != "" {
			sc.Released = strings.Split(videoObj.UploadDate, "T")[0]
		}

		// Tags
		e.ForEach(`.vd-tags__tag`, func(id int, e *colly.HTMLElement) {
			sc.Tags = append(sc.Tags, strings.TrimSpace(e.Text))
		})
		if scraperID == "virtualrealgay" {
			sc.Tags = append(sc.Tags, "Gay")
		}

		// Cast from pornstars list
		sc.ActorDetails = make(map[string]models.ActorDetails)
		e.ForEach(`.vd-pornstars__list .vd-pornstar__link`, func(_ int, el *colly.HTMLElement) {
			name := strings.TrimSpace(el.ChildText(".vd-pornstar__name"))
			if name == "" {
				return
			}
			name = strings.TrimSuffix(name, " VR")
			sc.Cast = append(sc.Cast, name)
			sc.ActorDetails[name] = models.ActorDetails{Source: scraperID + " scrape", ProfileUrl: strings.SplitN(e.Request.AbsoluteURL(el.Attr("href")), "?", 2)[0]}
		})

		// Cover URLs (prefer webp poster, fallback to jpg)
		e.ForEach(`.vdi-cover__poster source[type="image/webp"]`, func(id int, e *colly.HTMLElement) {
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
		e.ForEach(`.vdi-cover__poster img`, func(id int, e *colly.HTMLElement) {
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
		e.ForEach(`.vd-screenshots__grid a.vd-screenshots__item[data-gallery-src]`, func(id int, e *colly.HTMLElement) {
			u := e.Request.AbsoluteURL(strings.Split(e.Attr("data-gallery-src"), "?")[0])
			sc.Gallery = append(sc.Gallery, u)
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

		if sc.SceneID != "" && sc.Title != "" {
			out <- sc
		}
	})

	// List pages: discover pagination, then visit every page
	maxPage := 0
	siteIDRegex := regexp.MustCompile(`/videos/(\d+)/`)

	siteCollector.OnHTML(`.pagination-controls`, func(e *colly.HTMLElement) {
		if maxPage > 0 {
			return
		}
		e.ForEach(`.pagination-page`, func(_ int, el *colly.HTMLElement) {
			if n, err := strconv.Atoi(strings.TrimSpace(el.Text)); err == nil && n > maxPage {
				maxPage = n
			}
		})
		if maxPage > 1 && !limitScraping {
			for page := 2; page <= maxPage; page++ {
				siteCollector.Visit(URL + "videos/?page=" + strconv.Itoa(page))
			}
		}
	})

	// Scene cards on list pages
	siteCollector.OnHTML(`.card`, func(e *colly.HTMLElement) {
		sceneLink := e.ChildAttr("a.card-link", "href")
		if sceneLink == "" {
			return
		}
		sceneURL := strings.Split(e.Request.AbsoluteURL(sceneLink), "?")[0]

		if funk.ContainsString(knownScenes, sceneURL) {
			return
		}

		id := ""
		imgSrc := e.ChildAttr(".img picture img", "src")
		if imgSrc == "" {
			imgSrc = e.ChildAttr(".img picture source", "srcset")
		}
		if m := siteIDRegex.FindStringSubmatch(imgSrc); m != nil {
			id = m[1]
		}

		ctx := colly.NewContext()
		ctx.Put("siteID", id)
		sceneCollector.Request("GET", sceneURL, nil, ctx, nil)
	})

	if singleSceneURL != "" {
		sceneCollector.Visit(singleSceneURL)
	} else {
		siteCollector.Visit(URL + "videos/?page=1")
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
