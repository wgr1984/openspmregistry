package controller

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/utils"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (c *Controller) ListAction(w http.ResponseWriter, r *http.Request) {

	printCallInfo("List", r)

	if err := checkHeadersEnforce(r, "json"); err != nil {
		err.writeResponse(w)
		return // error already logged
	}

	// check scope name
	scope := r.PathValue("scope")
	packageName := utils.StripExtension(r.PathValue("package"), ".json")

	elements, err := listElements(w, c, r, scope, packageName)
	if err != nil {
		return // error already logged
	}

	header := w.Header()

	// latest-version link (spec 4.1) - add first so pagination can append
	addLinkHeaders(elements, "", c, header)

	// Pagination: ?page=N (optional per Swift Registry spec 4.1 example)
	page, perPage := parseListPagination(r, c.config.ListPageSize)
	var toRender []models.ListElement
	if perPage > 0 {
		toRender = paginateElements(elements, page, perPage)
		addListPaginationLinks(scope, packageName, len(elements), page, perPage, c, header)
	} else {
		toRender = elements
	}

	releaseList := make(map[string]models.Release)
	for _, element := range toRender {
		location := locationOfElement(c, element)
		releaseList[element.Version] = *models.NewRelease(location)
	}

	header.Set("Content-Version", "1")
	header.Set("Content-Type", mimetypes.ApplicationJson)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(models.NewListRelease(releaseList)); err != nil {
		slog.Error("Error encoding JSON:", "error", err)
	}
}

// parseListPagination reads page from query. Returns (page, perPage); perPage 0 means no pagination.
// When listPageSize is 0 (not configured), pagination is disabled and perPage is always 0.
// When listPageSize > 0, perPage is always pageSize; omitted or invalid ?page= is treated as page 1.
// The Swift Registry spec (4.1) exemplifies ?page=N in Link URLs but does not define query params.
func parseListPagination(r *http.Request, pageSize int) (page int, perPage int) {
	if pageSize <= 0 {
		return 1, 0
	}
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	return page, pageSize
}

// paginateElements returns the slice for the given page (1-based). 
// Elements are already sorted by precedence (highest first).
// If perPage is 0, all elements are returned.
// If page is less than 1, all elements are returned.
// If page is greater than the total number of pages, nil is returned.
func paginateElements(elements []models.ListElement, page int, perPage int) []models.ListElement {
	if perPage <= 0 || page < 1 {
		return elements
	}
	start := (page - 1) * perPage
	if start >= len(elements) {
		return nil
	}
	end := min(start+perPage, len(elements))
	return elements[start:end]
}

// addListPaginationLinks adds Link headers per Swift Registry spec 4.1:
// first, prev, next, last for paginated list responses. Uses ?page=N as in the spec example.
func addListPaginationLinks(scope, packageName string, totalCount, page, perPage int, c *Controller, header http.Header) {
	if perPage <= 0 || totalCount <= 0 {
		return
	}
	totalPages := (totalCount + perPage - 1) / perPage
	if totalPages <= 1 {
		return
	}
	if page > totalPages {
		page = totalPages
	}

	base := utils.BaseUrl(c.config)
	path, err := url.JoinPath(base, scope, packageName)
	if err != nil {
		slog.Error("Building pagination path", "error", err)
		return
	}
	makeLink := func(p int) string {
		return path + "?page=" + strconv.Itoa(p)
	}

	var links []string
	links = append(links, "<"+makeLink(1)+">; rel=\"first\"")
	if page > 1 {
		links = append(links, "<"+makeLink(page-1)+">; rel=\"prev\"")
	}
	if page < totalPages {
		links = append(links, "<"+makeLink(page+1)+">; rel=\"next\"")
	}
	links = append(links, "<"+makeLink(totalPages)+">; rel=\"last\"")

	existing := header.Get("Link")
	if existing != "" {
		header.Set("Link", existing+", "+strings.Join(links, ", "))
	} else {
		header.Set("Link", strings.Join(links, ", "))
	}
}
