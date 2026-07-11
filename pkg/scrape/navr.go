package scrape

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gocolly/colly/v2"
	"github.com/mozillazg/go-slugify"
	"github.com/nleeper/goment"
	"github.com/thoas/go-funk"
	"github.com/xbapps/xbvr/pkg/models"
)

type naSceneResponse struct {
	Access         int    `json:"access"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	ThumbnailImage string `json:"thumbnailImage"`
	Duration       int    `json:"duration"`
	Tags           []struct {
		Name string `json:"name"`
	} `json:"tags"`
}

func fetchNaughtyAmericaScene(sceneID string) (naSceneResponse, bool) {
	var result naSceneResponse

	req, err := http.NewRequest("POST", "https://api.naughtyapi.com/heresphere/"+sceneID, nil)
	if err != nil {
		return result, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return result, false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, false
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return result, false
	}

	if result.Access != 0 || result.Title == "" {
		return result, false
	}

	return result, true
}

func getNaughtyAmericaSceneID(sceneURL string) string {
	tmp := strings.Split(sceneURL, "-")
	return tmp[len(tmp)-1]
}

func parseNADuration(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	total := 0

	if h := regexp.MustCompile(`(\d+)\s*h`).FindStringSubmatch(s); h != nil {
		if n, err := strconv.Atoi(h[1]); err == nil {
			total += n * 60
		}
	}
	if m := regexp.MustCompile(`(\d+)\s*min`).FindStringSubmatch(s); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			total += n
		}
	}

	return total
}

func getNABasePrefixSlug(imageURL string) (string, string) {
	parts := strings.Split(imageURL, "/scenes/")
	if len(parts) < 2 {
		return "", ""
	}
	segs := strings.Split(parts[1], "/")
	if len(segs) < 2 {
		return "", ""
	}
	return segs[0], segs[1]
}

func buildNACovers(imageURL string) ([]string, []string) {
	var covers, gallery []string
	if imageURL == "" {
		return covers, gallery
	}

	imageURL = strings.Split(imageURL, "?")[0]
	m := regexp.MustCompile(`^(.*scene/)`).FindStringSubmatch(imageURL)
	if len(m) < 2 {
		return covers, gallery
	}
	base := m[1]

	covers = append(covers, imageURL)
	covers = append(covers, base+"vertical/1182x1788c.jpg")
	covers = append(covers, base+"horizontal/1182x777c.jpg")

	gallery = append(gallery, base+"image1/1182x777c.jpg")
	gallery = append(gallery, base+"image2/1000x563c.jpg")
	gallery = append(gallery, base+"image3/1000x563c.jpg")
	gallery = append(gallery, base+"image4/1000x563c.jpg")
	gallery = append(gallery, base+"image3/1279x852c.jpg")

	return covers, gallery
}

func NaughtyAmericaVR(wg *models.ScrapeWG, updateSite bool, knownScenes []string, out chan<- models.ScrapedScene, singleSceneURL string, singeScrapeAdditionalInfo string, limitScraping bool) error {
	defer wg.Done()
	scraperID := "naughtyamericavr"
	siteID := "NaughtyAmerica VR"
	logScrapeStart(scraperID, siteID)

	siteCollector := createCollector("www.naughtyamerica.com")
	sceneCollector := createCollector("www.naughtyamerica.com")

	maxPage := 0

	// Parse the pagination block once to discover the last page, then queue all remaining pages.
	siteCollector.OnHTML(`ul.pagination`, func(e *colly.HTMLElement) {
		if maxPage > 0 {
			return
		}
		e.ForEach(`li a`, func(_ int, el *colly.HTMLElement) {
			if n, err := strconv.Atoi(strings.TrimSpace(el.Text)); err == nil && n > maxPage {
				maxPage = n
			}
		})
		if maxPage > 1 && !limitScraping {
			for page := 2; page <= maxPage; page++ {
				siteCollector.Visit("https://www.naughtyamerica.com/vr-porn?page=" + strconv.Itoa(page))
			}
		}
	})

	// Scene cards on list pages
	siteCollector.OnHTML(`div.scene-item`, func(e *colly.HTMLElement) {
		sceneLink := e.ChildAttr("a.contain-img.vr-scene-item", "href")
		if sceneLink == "" {
			return
		}
		sceneURL := e.Request.AbsoluteURL(sceneLink)

		if funk.ContainsString(knownScenes, strings.Split(sceneURL, "?")[0]) {
			return
		}

		sceneCollector.Visit(sceneURL)
	})

	// Scene detail page
	sceneCollector.OnHTML(`html`, func(e *colly.HTMLElement) {
		sc := models.ScrapedScene{}
		sc.ScraperID = scraperID
		sc.SceneType = "VR"
		sc.Studio = "NaughtyAmerica"
		sc.Site = siteID
		sc.HomepageURL = strings.Split(e.Request.URL.String(), "?")[0]
		sc.MembersUrl = strings.Replace(sc.HomepageURL, "https://www.naughtyamerica.com/", "https://members.naughtyamerica.com/", 1)
		sc.SiteID = getNaughtyAmericaSceneID(sc.HomepageURL)
		sc.SceneID = slugify.Slugify(sc.Site) + "-" + sc.SiteID

		// Title: site title + scene title
		siteTitle := strings.TrimSpace(e.ChildText(`.site-title`))
		if siteTitle == "" {
			siteTitle = strings.TrimSpace(e.ChildText(`a.site-title`))
		}
		sceneTitle := strings.TrimSpace(e.ChildText(`.scene-title`))
		if sceneTitle == "" {
			sceneTitle = strings.TrimSpace(e.ChildText(`h1.scene-title`))
		}
		if siteTitle != "" && sceneTitle != "" {
			sc.Title = siteTitle + " " + sceneTitle
		} else if sceneTitle != "" {
			sc.Title = sceneTitle
		} else if siteTitle != "" {
			sc.Title = siteTitle
		}

		// Date
		dateText := strings.TrimSpace(e.ChildText(`.entry-date .light-grey-text`))
		if dateText == "" {
			dateText = strings.TrimSpace(e.ChildText(`div.entry-date`))
		}
		if dateText != "" {
			tmpDate, _ := goment.New(dateText, "MMM DD, YYYY")
			sc.Released = tmpDate.Format("YYYY-MM-DD")
		}

		// Duration
		durText := strings.TrimSpace(e.ChildText(`.duration .light-grey-text`))
		if durText == "" {
			durText = strings.TrimSpace(e.ChildText(`div.duration`))
		}
		sc.Duration = parseNADuration(durText)

		// Description
		sc.Synopsis = strings.TrimSpace(e.ChildText(`.video-description`))

		// Cast
		sc.ActorDetails = make(map[string]models.ActorDetails)
		e.ForEach(`.performer-list a`, func(_ int, el *colly.HTMLElement) {
			name := strings.TrimSpace(el.Text)
			if name == "" {
				return
			}
			sc.Cast = append(sc.Cast, name)
			sc.ActorDetails[name] = models.ActorDetails{Source: scraperID + " scrape", ProfileUrl: strings.SplitN(el.Request.AbsoluteURL(el.Attr("href")), "?", 2)[0]}
		})

		// Scene image from dl8-embed-container
		imageURL := e.ChildAttr(`.dl8-embed-container dl8-video`, "poster")
		if imageURL == "" {
			imageURL = e.ChildAttr(`.dl8-embed-container img`, "src")
		}
		if imageURL == "" {
			if m := regexp.MustCompile(`url\(["']?(.*?)["']?\)`).FindStringSubmatch(e.ChildAttr(`.dl8-embed-container`, "style")); m != nil {
				imageURL = m[1]
			}
		}

		// Fallback to Heresphere API for missing data and tags
		apiScene, apiOK := fetchNaughtyAmericaScene(sc.SiteID)
		if apiOK {
			if sc.Title == "" {
				sc.Title = apiScene.Title
			}
			if sc.Synopsis == "" {
				sc.Synopsis = apiScene.Description
			}
			if sc.Duration == 0 {
				sc.Duration = apiScene.Duration / 60000
			}
			if imageURL == "" {
				imageURL = apiScene.ThumbnailImage
			}
			for _, t := range apiScene.Tags {
				sc.Tags = append(sc.Tags, t.Name)
			}
		}

		// Skip scene if we could not extract the minimum required data
		if sc.Title == "" || imageURL == "" {
			return
		}

		sc.Covers, sc.Gallery = buildNACovers(imageURL)

		// Filenames
		prefix, slug := getNABasePrefixSlug(imageURL)
		if prefix != "" && slug != "" {
			baseName := prefix + slug
			defaultBaseName := "nam" + slug
			filenames := []string{"_180x180_3dh.mp4", "_smartphonevr60.mp4", "_smartphonevr30.mp4", "_vrdesktopsd.mp4", "_vrdesktophd.mp4", "_180_sbs.mp4", "_6kvr264.mp4", "_6kvr265.mp4", "_8kvr265.mp4"}
			for i := range filenames {
				sc.Filenames = append(sc.Filenames, baseName+filenames[i], defaultBaseName+filenames[i])
			}
		}

		// Trailer
		sc.TrailerType = "heresphere"
		params := models.TrailerScrape{SceneUrl: "https://api.naughtyapi.com/heresphere/" + sc.SiteID}
		strParams, _ := json.Marshal(params)
		sc.TrailerSrc = string(strParams)

		out <- sc
	})

	if singleSceneURL != "" {
		sceneCollector.Visit(singleSceneURL)
	} else {
		siteCollector.Visit("https://www.naughtyamerica.com/vr-porn")
	}

	if updateSite {
		updateSiteLastUpdate(scraperID)
	}
	logScrapeFinished(scraperID, siteID)
	return nil
}

func init() {
	registerScraper("naughtyamericavr", "NaughtyAmerica VR", "https://mcdn.vrporn.com/files/20170718100937/naughtyamericavr-vr-porn-studio-vrporn.com-virtual-reality.png", "naughtyamerica.com", NaughtyAmericaVR)
}
