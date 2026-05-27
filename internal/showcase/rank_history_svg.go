package showcase

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// SVG canvas and margin constants (pixels in viewBox coordinates).
// The legend panel is placed to the right of the plot area; svgMarginRight
// is wide enough to contain it so the plot width stays exactly svgPlotWidth.
const (
	svgViewHeight   = 560
	svgMarginLeft   = 55
	svgMarginTop    = 70
	svgMarginBottom = 50

	// Legend panel sits to the right of the plot area.
	svgLegendGap   = 25                            // gap between plot right edge and legend
	svgLegendCols  = 3                             // columns in the legend grid
	svgLegendColW  = 95                            // width per legend column (px)
	svgLegendWidth = svgLegendCols * svgLegendColW // 285 px total legend width

	// Plot area: explicitly sized so svgViewWidth is simply additive.
	svgPlotWidth   = 900
	svgMarginRight = svgLegendGap + svgLegendWidth + 15            // 325 px
	svgViewWidth   = svgMarginLeft + svgPlotWidth + svgMarginRight // 1280 px
)

// svgTimePoint is one weekly data snapshot for a project, embedded in the SVG
// for JavaScript tooltip rendering.
type svgTimePoint struct {
	Label string `json:"label"` // "now", "1w", "2w", …
	Spot  int    `json:"spot"`  // 0 means no data for this week
	Date  string `json:"date,omitempty"`
}

// svgProjectData carries per-project metadata used by the interactive JS layer.
// Inactive is true when the project's average commit age exceeds 730 days AND
// the last commit was over a year ago — matching the gemtext inactivity notice.
// Inactive projects are rendered as grey lines by default and only switch to
// their project colour when the user mouses over their legend entry.
type svgProjectData struct {
	Name     string         `json:"name"`
	Color    string         `json:"color"`
	Points   []svgTimePoint `json:"points"`
	Inactive bool           `json:"inactive"`
}

// projectColor returns a visually distinct CSS hex color for project index i.
// It uses golden-ratio hue spacing so successive projects never look similar.
func projectColor(i int) string {
	const golden = 0.618033988749895
	hue := math.Mod(float64(i)*golden*360, 360)
	return hslToRGBHex(hue, 0.75, 0.62)
}

// hslToRGBHex converts an HSL color (h in [0,360), s and l in [0,1]) to a
// CSS hex string like "#rrggbb".
func hslToRGBHex(h, s, l float64) string {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2

	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}

	ri := int(math.Round((r + m) * 255))
	gi := int(math.Round((g + m) * 255))
	bi := int(math.Round((b + m) * 255))
	return fmt.Sprintf("#%02x%02x%02x", ri, gi, bi)
}

// truncateName shortens s to at most maxRunes runes, appending "…" if cut.
func truncateName(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes-1]) + "…"
}

// xmlEscape replaces the characters that are special in SVG/XML text content.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// buildLegendSVG returns SVG markup for the 3-column project legend panel.
// Each entry calls onEnter/onLeave to synchronise with the plot lines.
// legendX is the left edge of the first legend column.
func buildLegendSVG(allProjects []svgProjectData, legendX, plotH int) string {
	if len(allProjects) == 0 {
		return ""
	}

	var buf strings.Builder

	// Faint vertical separator between plot and legend.
	fmt.Fprintf(&buf,
		`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#2a2a5a" stroke-width="1"/>`,
		legendX-12, svgMarginTop, legendX-12, svgMarginTop+plotH)

	// Legend header.
	fmt.Fprintf(&buf,
		`<text class="lhd" x="%d" y="%d">PROJECTS (hover to highlight)</text>`,
		legendX, svgMarginTop-8)

	// Distribute entries across columns: first fill column 0, then 1, then 2.
	rowsPerCol := (len(allProjects) + svgLegendCols - 1) / svgLegendCols
	const rowH = 13     // vertical stride per legend entry (px)
	const sqSize = 8    // colored square side length
	const maxChars = 12 // max display characters before truncation

	for i, proj := range allProjects {
		col := i / rowsPerCol
		row := i % rowsPerCol

		colX := legendX + col*svgLegendColW
		// rowY is the text baseline; the square is centered on it.
		rowY := svgMarginTop + row*rowH + rowH

		name := xmlEscape(truncateName(proj.Name, maxChars))

		// Each legend entry reuses onEnter/onLeave so hovering a legend item
		// highlights the corresponding plot line and opens the same tooltip.
		fmt.Fprintf(&buf,
			`<g class="lg" id="lg-%d" onmouseenter="onEnter(%d,event)" onmouseleave="onLeave()">`,
			i, i)
		fmt.Fprintf(&buf,
			`<rect class="lsq" x="%d" y="%d" width="%d" height="%d" fill="%s"/>`,
			colX, rowY-sqSize+1, sqSize, sqSize, proj.Color)
		fmt.Fprintf(&buf,
			`<text class="ltx" x="%d" y="%d">%s</text>`,
			colX+sqSize+3, rowY, name)
		buf.WriteString(`</g>`)
	}

	return buf.String()
}

// GenerateRankHistorySVG creates an interactive inline SVG that shows a
// Google-Trends-style rank history graph for all projects.
//
// Layout:
//   - Plot area: left = oldest snapshot, right = "now"; rank 1 at top.
//   - Legend panel: 3-column grid to the right of the plot; hovering a
//     legend entry highlights the corresponding plot line.
//   - The SVG uses width/height="100%" so it fills the browser window.
func GenerateRankHistorySVG(summaries []ProjectSummary) string {
	numPoints := rankHistoryPoints // up to 32 weekly snapshots

	// Collect per-project data, reversing the history so oldest is on the left.
	allProjects := make([]svgProjectData, 0, len(summaries))
	maxRank := 1
	colorIdx := 0

	for _, s := range summaries {
		if len(s.RankHistory) == 0 {
			continue
		}

		pts := make([]svgTimePoint, numPoints)
		hasData := false
		// RankHistory is newest-first (index 0 = "now", index len-1 = oldest).
		// We want pts to be oldest-first so the left side of the graph is
		// the most distant point.  Place each source entry at its destination
		// index (numPoints-1-j) so that "now" lands at pts[numPoints-1] and
		// older entries land to the left.  Entries for weeks with no snapshot
		// stay as zero-value svgTimePoint (Spot=0 → no data for that column).
		// Using j as the source index (not a reversed index based on numPoints)
		// prevents an out-of-bounds panic when len(s.RankHistory) < numPoints.
		for j := 0; j < len(s.RankHistory) && j < numPoints; j++ {
			dstIdx := numPoints - 1 - j // "now" (j=0) maps to rightmost slot
			h := s.RankHistory[j]
			pts[dstIdx] = svgTimePoint{
				Label: h.Anchor,
				Spot:  h.Spot,
				Date:  h.SnapshotDate,
			}
			if h.Spot > maxRank {
				maxRank = h.Spot
			}
			if h.Spot > 0 {
				hasData = true
			}
		}

		if !hasData {
			continue // skip projects that have never appeared in any snapshot
		}

		// A project is inactive when its average commit age (HEAD) exceeds 730
		// days AND no commit on ANY local branch is younger than 365 days.
		// Using LastActivityDate (all-branches) avoids false positives for
		// projects whose default branch is old but development continues on
		// another branch (e.g. a "develop" or "master" branch).
		// Code stats (AvgCommitAge, score) remain HEAD-only per config rules.
		inactive := false
		if s.Metadata != nil && s.Metadata.AvgCommitAge > 730 {
			activityDate := s.Metadata.LastActivityDate
			if activityDate == "" {
				activityDate = s.Metadata.LastCommitDate // fallback if field absent
			}
			if activityDate != "" {
				if last, err := time.Parse("2006-01-02", activityDate); err == nil {
					if time.Since(last).Hours()/24 > 365 {
						inactive = true
					}
				}
			}
		}

		allProjects = append(allProjects, svgProjectData{
			Name:     s.Name,
			Color:    projectColor(colorIdx),
			Points:   pts,
			Inactive: inactive,
		})
		colorIdx++
	}

	// Trim leading all-zero columns so the graph starts at the oldest week
	// that has real data for any project (not at week 32 if history only goes
	// back 5 weeks).  The rightmost column is always "now" (index numPoints-1).
	firstDataCol := numPoints - 1 // pessimistic: show at least "now"
outer:
	for col := 0; col < numPoints; col++ {
		for _, proj := range allProjects {
			if proj.Points[col].Spot > 0 {
				firstDataCol = col
				break outer
			}
		}
	}
	for i := range allProjects {
		allProjects[i].Points = allProjects[i].Points[firstDataCol:]
	}
	displayPoints := numPoints - firstDataCol // actual columns to render

	// Human-readable X-axis labels (left = oldest visible, right = "now").
	// Position i is (displayPoints-1-i) weeks ago; position displayPoints-1 is "now".
	xLabels := make([]string, displayPoints)
	for i := 0; i < displayPoints; i++ {
		weeksAgo := displayPoints - 1 - i
		if weeksAgo == 0 {
			xLabels[i] = "now"
		} else {
			xLabels[i] = fmt.Sprintf("%dw ago", weeksAgo)
		}
	}

	// --- Layout helpers ---
	plotW := svgViewWidth - svgMarginLeft - svgMarginRight // = svgPlotWidth = 900
	plotH := svgViewHeight - svgMarginTop - svgMarginBottom

	xPos := func(i int) float64 {
		if displayPoints <= 1 {
			return float64(svgMarginLeft) + float64(plotW)/2
		}
		return float64(svgMarginLeft) + float64(i)*float64(plotW)/float64(displayPoints-1)
	}

	// rank 1 → top of plot, maxRank → bottom of plot.
	yPos := func(rank int) float64 {
		if rank <= 0 {
			return -999 // off-screen sentinel; caller should skip
		}
		if maxRank <= 1 {
			return float64(svgMarginTop) + float64(plotH)/2
		}
		ratio := float64(rank-1) / float64(maxRank-1)
		return float64(svgMarginTop) + ratio*float64(plotH)
	}

	// Embed project data as JSON for the JS tooltip layer.
	projectsJSON, _ := json.Marshal(allProjects)

	// --- Build SVG sub-sections ---

	// Horizontal grid lines and Y-axis labels.
	var gridBuf strings.Builder
	yStep := gridStep(maxRank)
	plotRight := float64(svgMarginLeft + plotW)
	for r := 1; r <= maxRank; r += yStep {
		y := yPos(r)
		fmt.Fprintf(&gridBuf,
			`<line class="gl" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
			float64(svgMarginLeft), y, plotRight, y)
		fmt.Fprintf(&gridBuf,
			`<text class="al" x="%.1f" y="%.1f" text-anchor="end">%d</text>`,
			float64(svgMarginLeft)-6, y+4, r)
	}

	// Vertical grid lines and X-axis labels.
	// When there are many columns (long history), only label every Nth column
	// so the axis stays readable; "now" (rightmost) is always labelled.
	var xAxisBuf strings.Builder
	plotBottom := float64(svgMarginTop + plotH)
	labelStep := xLabelStep(displayPoints)
	for i := 0; i < displayPoints; i++ {
		x := xPos(i)
		fmt.Fprintf(&xAxisBuf,
			`<line class="gl" x1="%.1f" y1="%d" x2="%.1f" y2="%.1f"/>`,
			x, svgMarginTop, x, plotBottom)
		if i%labelStep == 0 || i == displayPoints-1 {
			fmt.Fprintf(&xAxisBuf,
				`<text class="al" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
				x, plotBottom+16, xLabels[i])
		}
	}

	// Project lines and dot groups.
	var linesBuf strings.Builder
	for i, proj := range allProjects {
		pathD := buildSVGPath(proj.Points, xPos, yPos)
		if pathD == "" {
			continue
		}

		// One circle per valid data point so the tooltip hit-area is larger.
		var circleBuf strings.Builder
		for j, pt := range proj.Points {
			if pt.Spot <= 0 {
				continue
			}
			x := xPos(j)
			y := yPos(pt.Spot)
			if y < 0 {
				continue
			}
			fmt.Fprintf(&circleBuf, `<circle cx="%.1f" cy="%.1f" r="4"/>`, x, y)
		}

		fmt.Fprintf(&linesBuf,
			`<g class="pg" id="pg-%d" onmouseenter="onEnter(%d,event)" onmouseleave="onLeave()">`,
			i, i)
		fmt.Fprintf(&linesBuf, `<path d="%s" stroke="%s" class="pl"/>`, pathD, proj.Color)
		fmt.Fprintf(&linesBuf, `<g class="pd" fill="%s">%s</g>`, proj.Color, circleBuf.String())
		linesBuf.WriteString(`</g>`)
	}

	// --- Assemble the full SVG ---
	var svg strings.Builder

	// position:fixed + top/left:0 overrides the browser's default body margin
	// so the SVG sits flush against all four edges of the viewport.  No width,
	// height, or viewBox is set here; rescale() writes them as explicit pixel
	// values derived from window.innerWidth/innerHeight so the chart always
	// fills the window without letterboxing.
	svg.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" style="position:fixed;top:0;left:0;display:block">`)

	// Embedded CSS – kept compact but readable.
	svg.WriteString(`<style>
svg{background:#1a1a2e;font-family:monospace}
.gl{stroke:#2a2a5a;stroke-width:.6;stroke-dasharray:4,3}
.al{fill:#888;font-size:11px}
.title{fill:#ddd;font-size:14px;font-weight:bold}
.sub{fill:#666;font-size:10px}
.pg{cursor:pointer;opacity:.55;transition:opacity .15s}
.pl{stroke-width:1;fill:none}
.pd circle{transition:r .15s}
.lg{cursor:pointer;opacity:1;transition:opacity .15s}
.ltx{fill:#aaa;font-size:9px}
.lhd{fill:#555;font-size:9px}
#tt{pointer-events:none;display:none}
#ttbg{fill:#1a1a40;stroke:#4444aa;stroke-width:1}
#tttl{fill:#fff;font-size:12px;font-weight:bold;font-family:monospace}
.ttrow{fill:#bbb;font-size:10px;font-family:monospace}
</style>`)

	// Solid background rect covers the whole viewport regardless of how the
	// chart group is transformed.  #chart is opened next so that rescale()
	// can reposition all chart elements together as a single unit.
	svg.WriteString(`<rect width="100%" height="100%" fill="#1a1a2e"/>`)
	svg.WriteString(`<g id="chart">`)

	// Title + two subtitle lines.
	fmt.Fprintf(&svg, `<text class="title" x="%d" y="28" text-anchor="middle">Project Rank History</text>`, svgViewWidth/2)
	fmt.Fprintf(&svg,
		`<text class="sub" x="%d" y="44" text-anchor="middle">rank 1 = highest score · hover a line or legend entry to highlight · %d projects tracked</text>`,
		svgViewWidth/2, len(allProjects))
	// Second subtitle: explain the grey lines so readers know what they mean.
	fmt.Fprintf(&svg,
		`<text class="sub" x="%d" y="57" text-anchor="middle">grey lines = inactive projects (no meaningful commits in 1+ years) · hover legend entry to highlight</text>`,
		svgViewWidth/2)

	// Rotated Y-axis label.
	cx := float64(svgMarginLeft) - 40
	cy := float64(svgMarginTop) + float64(plotH)/2
	fmt.Fprintf(&svg,
		`<text class="al" text-anchor="middle" transform="rotate(-90 %.1f %.1f)" x="%.1f" y="%.1f">Rank</text>`,
		cx, cy, cx, cy)

	// Grid, axes, and project lines.
	svg.WriteString(gridBuf.String())
	svg.WriteString(xAxisBuf.String())
	svg.WriteString(linesBuf.String())

	// Legend panel to the right of the plot area.
	legendX := svgMarginLeft + plotW + svgLegendGap
	svg.WriteString(buildLegendSVG(allProjects, legendX, plotH))

	// Tooltip overlay (hidden until a project is hovered).
	// #tttl is the project-name title; #ttbd holds the per-snapshot rows below it.
	svg.WriteString(`<g id="tt">
<rect id="ttbg" x="0" y="0" width="220" height="100" rx="5" ry="5"/>
<text id="tttl" x="10" y="18"></text>
<g id="ttbd"></g>
</g>`)

	// Close the #chart group before the script so all chart elements are
	// contained inside it and can be repositioned together by rescale().
	svg.WriteString(`</g>`)

	// Inline JavaScript for interactivity and dynamic scaling.
	// PROJECTS is the JSON array; each entry has name, color, and points[].
	// CHART_W / CHART_H are the fixed viewBox coordinates used when designing
	// the chart; rescale() maps them to the actual window size at runtime.
	fmt.Fprintf(&svg, `<script><![CDATA[
var PROJECTS=%s;
var CHART_W=%d, CHART_H=%d;
var svgEl=document.querySelector('svg');
var chartEl=document.getElementById('chart');
var tt=document.getElementById('tt');
var ttbg=document.getElementById('ttbg');
var tttl=document.getElementById('tttl');
var ttbd=document.getElementById('ttbd');
// allPG = plot line groups; allLG = legend entry groups (same count, same order).
// Query inside chartEl so the IDs are scoped to the chart group.
var allPG=chartEl.querySelectorAll('.pg');
var allLG=chartEl.querySelectorAll('.lg');
var activeIdx=-1;

// rescale stretches the chart to fill the full browser window in both axes.
// Sets explicit pixel width/height (bypassing the body-margin trap that makes
// percentage-relative sizes fall short in standalone SVG files) and a matching
// viewBox, then applies independent x/y scales so the chart always occupies
// every pixel — width tracks window width, height tracks window height.
// chartEl.getScreenCTM().inverse() accounts for both scale factors, keeping
// tooltip hit-testing correct regardless of the window aspect ratio.
function rescale(){
  var W=window.innerWidth||document.documentElement.clientWidth;
  var H=window.innerHeight||document.documentElement.clientHeight;
  svgEl.setAttribute('width',W);
  svgEl.setAttribute('height',H);
  svgEl.setAttribute('viewBox','0 0 '+W+' '+H);
  var sx=W/CHART_W, sy=H/CHART_H;
  chartEl.setAttribute('transform','scale('+sx+','+sy+')');
}
window.addEventListener('resize',rescale);
rescale();

// Initialise inactive projects: grey plot line, dimmed legend entry.
// This runs once after DOM and rescale() are ready.
for(var i=0;i<allPG.length;i++){
  if(PROJECTS[i].inactive){
    allPG[i].style.opacity='0.3';
    allPG[i].querySelector('.pl').style.stroke='#555';
  }
}
for(var i=0;i<allLG.length;i++){
  if(PROJECTS[i].inactive) allLG[i].style.opacity='0.35';
}

// defaultPGOpacity returns the resting opacity for a plot group.
function defaultPGOpacity(i){return PROJECTS[i].inactive?'0.3':'0.55';}
// defaultLGOpacity returns the resting opacity for a legend entry.
function defaultLGOpacity(i){return PROJECTS[i].inactive?'0.35':'1';}

// onEnter is called when the cursor enters a project group or legend entry.
// It dims/greys all other groups, shows the tooltip, and marks the project active.
function onEnter(idx,evt){
  activeIdx=idx;
  var p=PROJECTS[idx];
  tttl.textContent=p.name;

  // Clear old tooltip rows.
  while(ttbd.firstChild)ttbd.removeChild(ttbd.firstChild);

  // Build one row per snapshot, newest first (points array is oldest-first).
  // Start below the title (baseline y=18 + gap) so rows never overlap the title.
  var y=32;
  for(var i=p.points.length-1;i>=0;i--){
    var pt=p.points[i];
    if(pt.spot<=0)continue;
    var label=pt.label==='now'?'now':pt.label+' ago';
    var line='#'+pt.spot+' '+label+(pt.date?' ('+pt.date+')':'');
    var t=document.createElementNS('http://www.w3.org/2000/svg','text');
    t.setAttribute('x','10');
    t.setAttribute('y',String(y));
    t.setAttribute('class','ttrow');
    t.textContent=line;
    ttbd.appendChild(t);
    y+=13;
  }

  // Resize tooltip background: width adapts to the project name, height to rows.
  var nameW=p.name.length*7.5+20;
  var w=Math.max(160,Math.min(300,nameW));
  var h=Math.max(36,y+8);
  ttbg.setAttribute('width',w);
  ttbg.setAttribute('height',h);

  // Apply per-project opacity for plot lines:
  //   - hovered project → full opacity + project colour (restores inactive grey)
  //   - active others   → dimmed to 0.08
  //   - inactive others → stay at their grey resting state (not dimmed further)
  for(var i=0;i<allPG.length;i++){
    var pl=allPG[i].querySelector('.pl');
    if(i===idx){
      allPG[i].style.opacity='1';
      pl.style.stroke=PROJECTS[i].color; // restore colour for inactive projects
      pl.style.strokeWidth='3';
    } else if(PROJECTS[i].inactive){
      // Keep inactive lines at their grey resting state so they do not compete.
      allPG[i].style.opacity='0.3';
      pl.style.stroke='#555';
    } else {
      allPG[i].style.opacity='0.08';
    }
  }

  // Highlight hovered legend entry; dim all others uniformly.
  for(var i=0;i<allLG.length;i++){
    allLG[i].style.opacity=(i===idx)?'1':'0.2';
  }

  moveTT(evt);
  tt.style.display='block';
}

// onLeave restores all groups to their per-project resting state.
function onLeave(){
  tt.style.display='none';
  for(var i=0;i<allPG.length;i++){
    var pl=allPG[i].querySelector('.pl');
    allPG[i].style.opacity=defaultPGOpacity(i);
    pl.style.strokeWidth='';
    // Restore grey stroke for inactive projects; clear override for active ones.
    pl.style.stroke=PROJECTS[i].inactive?'#555':'';
  }
  for(var i=0;i<allLG.length;i++){
    allLG[i].style.opacity=defaultLGOpacity(i);
  }
  activeIdx=-1;
}

// Follow the cursor while hovering.
svgEl.addEventListener('mousemove',function(evt){
  if(activeIdx>=0)moveTT(evt);
});

// moveTT repositions the tooltip near the cursor, keeping it inside the chart
// coordinate space (CHART_W × CHART_H).  chartEl.getScreenCTM() accounts for
// the rescale() transform, so the returned point is already in chart coords.
function moveTT(evt){
  var pt=svgEl.createSVGPoint();
  pt.x=evt.clientX; pt.y=evt.clientY;
  var sp=pt.matrixTransform(chartEl.getScreenCTM().inverse());
  var w=parseFloat(ttbg.getAttribute('width'));
  var h=parseFloat(ttbg.getAttribute('height'));
  var tx=sp.x+14, ty=sp.y-10;
  if(tx+w>CHART_W-5)tx=sp.x-w-14;
  if(ty+h>CHART_H-5)ty=CHART_H-h-5;
  if(ty<5)ty=5;
  tt.setAttribute('transform','translate('+tx+','+ty+')');
}
]]></script>`, string(projectsJSON), svgViewWidth, svgViewHeight)

	svg.WriteString(`</svg>`)
	return svg.String()
}

// buildSVGPath converts a slice of time points into an SVG path string using
// M (moveto) at the start of each run of valid points and L (lineto) within.
// Gaps (Spot == 0) cause the pen to be lifted so the line breaks cleanly.
func buildSVGPath(points []svgTimePoint, xPos func(int) float64, yPos func(int) float64) string {
	var parts []string
	prevValid := false

	for i, pt := range points {
		if pt.Spot <= 0 {
			prevValid = false
			continue
		}
		x := xPos(i)
		y := yPos(pt.Spot)
		if y < 0 {
			prevValid = false
			continue
		}
		cmd := "L"
		if !prevValid {
			cmd = "M"
		}
		parts = append(parts, fmt.Sprintf("%s%.1f %.1f", cmd, x, y))
		prevValid = true
	}

	return strings.Join(parts, " ")
}

// gridStep returns how many rank positions to skip between Y-axis grid lines,
// keeping the graph legible when there are many projects.
func gridStep(maxRank int) int {
	switch {
	case maxRank > 30:
		return 5
	case maxRank > 15:
		return 3
	case maxRank > 8:
		return 2
	default:
		return 1
	}
}

// xLabelStep returns how many X-axis columns to skip between printed labels so
// the time axis stays readable when many weeks of history are displayed.
// Grid lines are always drawn at every column; only labels are thinned.
func xLabelStep(displayPoints int) int {
	switch {
	case displayPoints > 24:
		return 8
	case displayPoints > 12:
		return 4
	case displayPoints > 6:
		return 2
	default:
		return 1
	}
}
