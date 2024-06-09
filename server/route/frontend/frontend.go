package frontend

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/yourselfhosted/slash/internal/util"
	storepb "github.com/yourselfhosted/slash/proto/gen/store"
	"github.com/yourselfhosted/slash/server/metric"
	"github.com/yourselfhosted/slash/server/profile"
	"github.com/yourselfhosted/slash/store"
)

//go:embed dist
var embeddedFiles embed.FS

const (
	headerMetadataPlaceholder = "<!-- slash.metadata -->"
)

type FrontendService struct {
	Profile *profile.Profile
	Store   *store.Store
}

func NewFrontendService(profile *profile.Profile, store *store.Store) *FrontendService {
	return &FrontendService{
		Profile: profile,
		Store:   store,
	}
}

func validatePrefix(p string) error {
	// Canonicalize the prefix in the form "/s"
	p = path.Join("/", p, "/")

	// Special prefixes, including anything that starts with /slash (for future use)
	badPrefixes := []string{
		"/api", "/slash", "/robots.txt", "/sitemap.xml", "/crossdomain.xml", "/favicon.ico",
	}
	if util.HasPrefixes(p, badPrefixes...) || p == "/c" {
		return fmt.Errorf("Invalid shortcut prefix %q", p)
	}
	return nil
}

func (s *FrontendService) Serve(ctx context.Context, e *echo.Echo) error {
	// Use echo static middleware to serve the built dist folder.
	// Reference: https://github.com/labstack/echo/blob/master/middleware/static.go
	prefix := "s"
	if err := validatePrefix(prefix); err != nil {
		return err
	}
	shortcutPath := path.Join("/", prefix, ":shortcutName")

	skipper := func(c echo.Context) bool {
		return util.HasPrefixes(c.Path(), "/api", "/slash.api.v1", "/robots.txt", "/sitemap.xml", shortcutPath, "/c/:collectionName")
	}
	e.Use(middleware.StaticWithConfig(middleware.StaticConfig{
		HTML5:      true,
		Filesystem: getFileSystem("dist"),
		Skipper:    skipper,
	}))

	g := e.Group("assets")
	// Use echo gzip middleware to compress the response.
	// Reference: https://echo.labstack.com/docs/middleware/gzip
	g.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Skipper: skipper,
		Level:   5,
	}))
	g.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set(echo.HeaderCacheControl, "max-age=31536000, immutable")
			return next(c)
		}
	})
	g.Use(middleware.StaticWithConfig(middleware.StaticConfig{
		HTML5:      true,
		Filesystem: getFileSystem("dist/assets"),
		Skipper:    skipper,
	}))

	s.registerRoutes(e, shortcutPath)
	s.registerFileRoutes(ctx, e, shortcutPath)
	return nil
}

func (s *FrontendService) registerRoutes(e *echo.Echo, shortcutPath string) {
	rawIndexHTML := getRawIndexHTML()

	e.GET(shortcutPath, func(c echo.Context) error {
		ctx := c.Request().Context()
		shortcutName := c.Param("shortcutName")
		shortcut, err := s.Store.GetShortcut(ctx, &store.FindShortcut{
			Name: &shortcutName,
		})
		if err != nil {
			return c.HTML(http.StatusOK, rawIndexHTML)
		}
		if shortcut == nil {
			return c.HTML(http.StatusOK, rawIndexHTML)
		}

		metric.Enqueue("shortcut view")
		// Inject shortcut metadata into `index.html`.
		indexHTML := strings.ReplaceAll(rawIndexHTML, headerMetadataPlaceholder, generateShortcutMetadata(shortcut).String())
		return c.HTML(http.StatusOK, indexHTML)
	})

	e.GET("/c/:collectionName", func(c echo.Context) error {
		ctx := c.Request().Context()
		collectionName := c.Param("collectionName")
		collection, err := s.Store.GetCollection(ctx, &store.FindCollection{
			Name: &collectionName,
		})
		if err != nil {
			return c.HTML(http.StatusOK, rawIndexHTML)
		}
		if collection == nil {
			return c.HTML(http.StatusOK, rawIndexHTML)
		}

		metric.Enqueue("collection view")
		// Inject collection metadata into `index.html`.
		indexHTML := strings.ReplaceAll(rawIndexHTML, headerMetadataPlaceholder, generateCollectionMetadata(collection).String())
		return c.HTML(http.StatusOK, indexHTML)
	})
}

func (s *FrontendService) registerFileRoutes(ctx context.Context, e *echo.Echo, shortcutPrefix string) {
	instanceURLSetting, err := s.Store.GetWorkspaceSetting(ctx, &store.FindWorkspaceSetting{
		Key: storepb.WorkspaceSettingKey_WORKSPACE_SETTING_INSTANCE_URL,
	})
	if err != nil || instanceURLSetting == nil {
		return
	}
	instanceURL := instanceURLSetting.GetInstanceUrl()
	if instanceURL == "" {
		return
	}

	e.GET("/robots.txt", func(c echo.Context) error {
		robotsTxt := fmt.Sprintf(`User-agent: *
Allow: /
Host: %s
Sitemap: %s/sitemap.xml`, instanceURL, instanceURL)
		return c.String(http.StatusOK, robotsTxt)
	})

	e.GET("/sitemap.xml", func(c echo.Context) error {
		urlsets := []string{}
		// Append shortcut list.
		shortcuts, err := s.Store.ListShortcuts(ctx, &store.FindShortcut{
			VisibilityList: []store.Visibility{store.VisibilityPublic},
		})
		if err != nil {
			return err
		}
		for _, shortcut := range shortcuts {
			urlsets = append(urlsets, fmt.Sprintf(`<url><loc>%s</loc></url>`, path.Join(instanceURL, shortcutPrefix, shortcut.Name)))
		}
		// Append collection list.
		collections, err := s.Store.ListCollections(ctx, &store.FindCollection{
			VisibilityList: []store.Visibility{store.VisibilityPublic},
		})
		if err != nil {
			return err
		}
		for _, collection := range collections {
			urlsets = append(urlsets, fmt.Sprintf(`<url><loc>%s/c/%s</loc></url>`, instanceURL, collection.Name))
		}

		sitemap := fmt.Sprintf(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:news="http://www.google.com/schemas/sitemap-news/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml" xmlns:mobile="http://www.google.com/schemas/sitemap-mobile/1.0" xmlns:image="http://www.google.com/schemas/sitemap-image/1.1" xmlns:video="http://www.google.com/schemas/sitemap-video/1.1">%s</urlset>`, strings.Join(urlsets, "\n"))
		return c.XMLBlob(http.StatusOK, []byte(sitemap))
	})
}

func getFileSystem(path string) http.FileSystem {
	fs, err := fs.Sub(embeddedFiles, path)
	if err != nil {
		panic(err)
	}

	return http.FS(fs)
}

func generateShortcutMetadata(shortcut *storepb.Shortcut) *Metadata {
	metadata := getDefaultMetadata()
	title, description := shortcut.Title, shortcut.Description
	if shortcut.OgMetadata != nil {
		if shortcut.OgMetadata.Title != "" {
			title = shortcut.OgMetadata.Title
		}
		if shortcut.OgMetadata.Description != "" {
			description = shortcut.OgMetadata.Description
		}
		metadata.ImageURL = shortcut.OgMetadata.Image
	}
	metadata.Title = title
	metadata.Description = description
	return metadata
}

func generateCollectionMetadata(collection *storepb.Collection) *Metadata {
	metadata := getDefaultMetadata()
	metadata.Title = collection.Title
	metadata.Description = collection.Description
	return metadata
}

func getRawIndexHTML() string {
	bytes, _ := embeddedFiles.ReadFile("dist/index.html")
	return string(bytes)
}

type Metadata struct {
	Title       string
	Description string
	ImageURL    string
}

func getDefaultMetadata() *Metadata {
	return &Metadata{
		Title: "Slash",
	}
}

func (m *Metadata) String() string {
	metadataList := []string{
		fmt.Sprintf(`<title>%s</title>`, m.Title),
		fmt.Sprintf(`<meta name="description" content="%s" />`, m.Description),
		fmt.Sprintf(`<meta property="og:title" content="%s" />`, m.Title),
		fmt.Sprintf(`<meta property="og:description" content="%s" />`, m.Description),
		fmt.Sprintf(`<meta property="og:image" content="%s" />`, m.ImageURL),
		`<meta property="og:type" content="website" />`,
		// Twitter related fields.
		fmt.Sprintf(`<meta property="twitter:title" content="%s" />`, m.Title),
		fmt.Sprintf(`<meta property="twitter:description" content="%s" />`, m.Description),
		fmt.Sprintf(`<meta property="twitter:image" content="%s" />`, m.ImageURL),
	}
	return strings.Join(metadataList, "\n")
}
