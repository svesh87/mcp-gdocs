package tools

import (
	"strings"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// chartOnTab is a chart whose ranges read one tab and stop at a fixed row there.
//
// A chart's range is written down once, when the chart is drawn, as a rectangle of the
// grid. Rows added under that rectangle are rows the chart has never heard of: the tab
// grows, the chart stays, and the deck that shows it goes on presenting last month as the
// latest. Nothing in either API reports this, so the tool that adds the rows has to.
type chartOnTab struct {
	chartID int
	title   string
	spec    map[string]any
	// sources are the rectangles on this tab, as they sit inside spec: changing one here
	// changes the specification that is about to be sent back.
	sources []map[string]any
	// endRow is the row the lowest of them stops before, counted from 0.
	endRow int
	// sameEnd says every one of them stops before that same row, which is what a chart
	// drawn from one block of data looks like.
	sameEnd bool
}

// chartsReadingTab picks out the charts with a bounded range on one tab.
//
// The ranges are found by walking the whole specification rather than by knowing where
// each kind of chart keeps them: a basic chart has domains and series, a pie has one of
// each, a candlestick has four per candle, and every one of them ends in the same shape —
// "sources", a list of grid ranges. A range with no end row runs to the bottom of the tab
// and grows by itself; it is not reported.
func chartsReadingTab(charts []google.StoredChart, sheetID int) []chartOnTab {
	var reading []chartOnTab

	for _, chart := range charts {
		var all []map[string]any
		collectSources(chart.Spec, &all)

		found := chartOnTab{chartID: chart.ChartID, spec: chart.Spec, sameEnd: true}
		found.title, _ = chart.Spec["title"].(string)

		for _, source := range all {
			// Sheets leaves a zero out of its answers, so a range with no sheetId is on
			// the tab numbered 0 — the first tab of nearly every workbook.
			onSheet, _ := numberField(source, "sheetId")
			if onSheet != sheetID {
				continue
			}
			end, bounded := numberField(source, "endRowIndex")
			if !bounded {
				continue
			}

			if len(found.sources) > 0 && end != found.endRow {
				found.sameEnd = false
			}
			if end > found.endRow {
				found.endRow = end
			}
			found.sources = append(found.sources, source)
		}

		if len(found.sources) > 0 {
			reading = append(reading, found)
		}
	}

	return reading
}

// collectSources gathers every grid range a specification draws from.
func collectSources(node any, into *[]map[string]any) {
	switch typed := node.(type) {
	case map[string]any:
		for key, value := range typed {
			if list, ok := value.([]any); ok && key == "sources" {
				for _, item := range list {
					if source, ok := item.(map[string]any); ok {
						*into = append(*into, source)
					}
				}
				continue
			}
			collectSources(value, into)
		}
	case []any:
		for _, item := range typed {
			collectSources(item, into)
		}
	}
}

// numberField reads a whole number out of decoded JSON, where every number is a float.
func numberField(object map[string]any, name string) (int, bool) {
	switch typed := object[name].(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	}

	return 0, false
}

// extendTo moves every range of the chart on this tab to stop before a new row.
func (c *chartOnTab) extendTo(endRow int) {
	for _, source := range c.sources {
		source["endRowIndex"] = endRow
	}
}

// quotedSheet writes a tab's name the way an A1 range takes it: in single quotes, with a
// quote inside doubled. Unquoted, a name with a space or one that looks like a cell — a tab
// called Q1 — is read as something else.
func quotedSheet(title string) string {
	return "'" + strings.ReplaceAll(title, "'", "''") + "'"
}
