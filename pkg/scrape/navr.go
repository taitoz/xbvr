package scrape

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/mozillazg/go-slugify"
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

type naScenesResponse struct {
	CurrentPage int          `json:"current_page"`
	LastPage    int          `json:"last_page"`
	Total       int          `json:"total"`
	Data        []naSceneAPI `json:"data"`
}

type naSceneAPI struct {
	ID             int                 `json:"id"`
	Title          string              `json:"title"`
	Length         int                 `json:"length"`
	PublishedDate  string              `json:"published_date"`
	SceneURL       string              `json:"scene_url"`
	SiteName       string              `json:"site_name"`
	Synopsis       string              `json:"synopsis"`
	Tags           []string            `json:"tags"`
	Performers     map[string][]string `json:"performers"`
	Trailers       map[string]string   `json:"trailers"`
	PromoVideoData map[string]string   `json:"promo_video_data"`
}

func hasVRTag(tags []string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, "Virtual Reality") || strings.EqualFold(t, "VR Porn") {
			return true
		}
	}
	return false
}

func fetchNAVRPages(limitScraping bool) ([]naSceneAPI, error) {
	var all []naSceneAPI
	page := 1
	for {
		if limitScraping && page > 1 {
			break
		}
		u := fmt.Sprintf("https://api.naughtyapi.com/tools/scenes/scenes?page=%d", page)
		resp, err := http.Get(u)
		if err != nil {
			return all, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			break
		}

		var r naScenesResponse
		if err := json.Unmarshal(body, &r); err != nil {
			break
		}

		for _, s := range r.Data {
			if !hasVRTag(s.Tags) {
				continue
			}
			if !strings.Contains(s.SceneURL, "www.naughtyamerica.com") {
				continue
			}
			all = append(all, s)
		}

		if r.LastPage == 0 || page >= r.LastPage {
			break
		}
		page++
	}
	return all, nil
}

func parseNADate(s string) string {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"Jan 2, 2006", "January 2, 2006", "01/02/2006", "01-02-2006", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

func getNACoverFromAPI(scene naSceneAPI) string {
	// Combine trailers and promo video URLs, prefer promo data for cover extraction
	combined := make(map[string]string)
	for k, v := range scene.Trailers {
		combined[k] = v
	}
	for k, v := range scene.PromoVideoData {
		combined[k] = v
	}
	if len(combined) == 0 {
		return ""
	}

	var videoURL string
	for _, v := range combined {
		if v != "" {
			videoURL = v
			break
		}
	}
	if videoURL == "" {
		return ""
	}

	re := regexp.MustCompile(`.+(?:promo|\.com)/(?:nonsecure/)?([^/]+)/(?:trailers(?:/vr)?/)?([^/_]+)`)
	m := re.FindStringSubmatch(videoURL)
	if len(m) < 3 {
		return ""
	}
	prefix := m[1]
	name := m[2]
	if strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
		name = name[len(prefix):]
	}
	name = regexp.MustCompile(`(?i)(teaser|trailer)$`).ReplaceAllString(name, "")
	if name == "" || prefix == "" {
		return ""
	}

	resolution := "1279x852"
	return fmt.Sprintf("https://images4.naughtycdn.com/cms/nacmscontent/v1/scenes/%s/%s/scene/horizontal/%sc.jpg", prefix, name, resolution)
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

	sceneCollector := createCollector("www.naughtyamerica.com")

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

		// Load the API scene data passed from the listing
		var apiScene naSceneAPI
		if apiJSON := e.Request.Ctx.Get("apiScene"); apiJSON != "" {
			_ = json.Unmarshal([]byte(apiJSON), &apiScene)
		}

		// Title: site title + scene title
		siteTitle := strings.TrimSpace(e.ChildText(`.site-title`))
		if siteTitle == "" {
			siteTitle = strings.TrimSpace(e.ChildText(`a.site-title`))
		}
		if siteTitle == "" {
			siteTitle = strings.TrimSpace(e.ChildText(`[class*="site-title"]`))
		}
		if siteTitle == "" {
			siteTitle = apiScene.SiteName
		}

		sceneTitle := strings.TrimSpace(e.ChildText(`.scene-title`))
		if sceneTitle == "" {
			sceneTitle = strings.TrimSpace(e.ChildText(`h1.scene-title`))
		}
		if sceneTitle == "" {
			sceneTitle = strings.TrimSpace(e.ChildText(`[class*="scene-title"]`))
		}
		if sceneTitle == "" {
			sceneTitle = apiScene.Title
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
		if dateText == "" && apiScene.PublishedDate != "" {
			dateText = strings.Split(apiScene.PublishedDate, " ")[0]
		}
		if dateText != "" {
			sc.Released = parseNADate(dateText)
		}

		// Duration
		durText := strings.TrimSpace(e.ChildText(`.duration .light-grey-text`))
		if durText == "" {
			durText = strings.TrimSpace(e.ChildText(`div.duration`))
		}
		sc.Duration = parseNADuration(durText)
		if sc.Duration == 0 && apiScene.Length > 0 {
			sc.Duration = apiScene.Length / 60
		}

		// Description
		sc.Synopsis = strings.TrimSpace(e.ChildText(`.video-description`))
		if sc.Synopsis == "" {
			sc.Synopsis = strings.TrimSpace(apiScene.Synopsis)
		}

		// Cast from performer list items
		sc.ActorDetails = make(map[string]models.ActorDetails)
		added := make(map[string]bool)
		tryCast := func(name, profileURL string) {
			name = strings.TrimSpace(name)
			if name == "" || added[name] {
				return
			}
			added[name] = true
			sc.Cast = append(sc.Cast, name)
			sc.ActorDetails[name] = models.ActorDetails{Source: scraperID + " scrape", ProfileUrl: strings.SplitN(profileURL, "?", 2)[0]}
		}
		e.ForEach(`.performer-list li`, func(_ int, li *colly.HTMLElement) {
			name := strings.TrimSpace(li.ChildText("a"))
			if name == "" {
				name = strings.TrimSpace(li.Text)
			}
			tryCast(name, e.Request.AbsoluteURL(li.ChildAttr("a", "href")))
		})
		// Fallback to any anchor inside the performer list
		if len(sc.Cast) == 0 {
			e.ForEach(`.performer-list a`, func(_ int, el *colly.HTMLElement) {
				tryCast(el.Text, e.Request.AbsoluteURL(el.Attr("href")))
			})
		}
		// Fallback to API performers
		if len(sc.Cast) == 0 {
			for _, names := range apiScene.Performers {
				for _, name := range names {
					tryCast(name, "")
				}
			}
		}

		// Tags from API
		sc.Tags = apiScene.Tags

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
		if imageURL == "" {
			imageURL = getNACoverFromAPI(apiScene)
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
		apiScenes, err := fetchNAVRPages(limitScraping)
		if err != nil {
			log.Errorf("Error fetching NaughtyAmerica scene list: %v", err)
		}
		for _, s := range apiScenes {
			sceneURL := strings.Split(s.SceneURL, "?")[0]
			if funk.ContainsString(knownScenes, sceneURL) {
				continue
			}
			apiJSON, _ := json.Marshal(s)
			ctx := colly.NewContext()
			ctx.Put("apiScene", string(apiJSON))
			sceneCollector.Request("GET", sceneURL, nil, ctx, nil)
		}
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
