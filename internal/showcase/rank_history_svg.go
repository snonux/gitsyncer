package showcase

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// SVG canvas and margin constants (pixels in viewBox coordinates).
const (
	svgViewWidth    = 1000
	svgViewHeight   = 560
	svgMarginLeft   = 55
	svgMarginRight  = 20
	svgMarginTop    = 70
	svgMarginBottom = 50
)

// svgTimePoint is one weekly data snapshot for a project, embedded in the SVG
// for JavaScript tooltip rendering.
type svgTimePoint struct {
	Label string `json:"label"` // "now", "1w", "2w", …
	Spot  int    `json:"spot"`  // 0 means no data for this week
	Date  string `json:"date,omitempty"`
}

// svgProjectData carries per-project metadata used by the interactive JS layer.
type svgProjectData struct {
	Name   string         `json:"name"`
	Color  string         `json:"color"`
	Points []svgTimePoint `json:"points"`
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

// GenerateRankHistorySVG creates an interactive inline SVG that shows a
// Google-Trends-style rank history graph for all projects.
//
// Layout:
//   - X-axis: left = oldest snapshot, right = "now".
//   - Y-axis: rank 1 at top (best), max rank at bottom.
//   - Each project is one colored polyline; gaps occur when a week has no data.
//   - Hovering a line highlights it, dims others, and shows a tooltip with the
//     project name and rank at each recorded snapshot.
func GenerateRankHistorySVG(summaries []ProjectSummary) string {
	numPoints := rankHistoryPoints // 5 weekly snapshots

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

		allProjects = append(allProjects, svgProjectData{
			Name:   s.Name,
			Color:  projectColor(colorIdx),
			Points: pts,
		})
		colorIdx++
	}

	// Human-readable X-axis labels (left = oldest, right = "now").
	// Position i is (numPoints-1-i) weeks ago; position numPoints-1 is "now".
	xLabels := make([]string, numPoints)
	for i := 0; i < numPoints; i++ {
		weeksAgo := numPoints - 1 - i
		if weeksAgo == 0 {
			xLabels[i] = "now"
		} else {
			xLabels[i] = fmt.Sprintf("%dw ago", weeksAgo)
		}
	}

	// --- Layout helpers ---
	plotW := svgViewWidth - svgMarginLeft - svgMarginRight
	plotH := svgViewHeight - svgMarginTop - svgMarginBottom

	xPos := func(i int) float64 {
		if numPoints <= 1 {
			return float64(svgMarginLeft) + float64(plotW)/2
		}
		return float64(svgMarginLeft) + float64(i)*float64(plotW)/float64(numPoints-1)
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
	var xAxisBuf strings.Builder
	plotBottom := float64(svgMarginTop + plotH)
	for i := 0; i < numPoints; i++ {
		x := xPos(i)
		fmt.Fprintf(&xAxisBuf,
			`<line class="gl" x1="%.1f" y1="%d" x2="%.1f" y2="%.1f"/>`,
			x, svgMarginTop, x, plotBottom)
		fmt.Fprintf(&xAxisBuf,
			`<text class="al" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
			x, plotBottom+16, xLabels[i])
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

	fmt.Fprintf(&svg,
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		svgViewWidth, svgViewHeight, svgViewWidth, svgViewHeight)

	// Embedded CSS – kept compact but readable.
	svg.WriteString(`<style>
svg{background:#1a1a2e;font-family:monospace}
.gl{stroke:#2a2a5a;stroke-width:.6;stroke-dasharray:4,3}
.al{fill:#888;font-size:11px}
.title{fill:#ddd;font-size:14px;font-weight:bold}
.sub{fill:#666;font-size:10px}
.pg{cursor:pointer;opacity:.55;transition:opacity .15s}
.pl{stroke-width:1.8;fill:none}
.pd circle{transition:r .15s}
#tt{pointer-events:none;display:none}
#ttbg{fill:#1a1a40;stroke:#4444aa;stroke-width:1}
#tttl{fill:#fff;font-size:12px;font-weight:bold;font-family:monospace}
.ttrow{fill:#bbb;font-size:10px;font-family:monospace}
</style>`)

	// Solid background rect (belt-and-suspenders for SVG viewers that ignore
	// the CSS background property).
	fmt.Fprintf(&svg, `<rect width="%d" height="%d" fill="#1a1a2e"/>`, svgViewWidth, svgViewHeight)

	// Title + subtitle.
	fmt.Fprintf(&svg, `<text class="title" x="%d" y="28" text-anchor="middle">Project Rank History</text>`, svgViewWidth/2)
	fmt.Fprintf(&svg,
		`<text class="sub" x="%d" y="44" text-anchor="middle">rank 1 = highest score · hover a line to highlight · %d projects tracked</text>`,
		svgViewWidth/2, len(allProjects))

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

	// Tooltip overlay (hidden until a project is hovered).
	svg.WriteString(`<g id="tt">
<rect id="ttbg" x="0" y="0" width="220" height="100" rx="5" ry="5"/>
<text id="tttl" x="10" y="18"></text>
<g id="ttbd"></g>
</g>`)

	// Inline JavaScript for interactivity.
	// PROJECTS is the JSON array; each entry has name, color, and points[].
	fmt.Fprintf(&svg, `<script><![CDATA[
var PROJECTS=%s;
var svgEl=document.querySelector('svg');
var tt=document.getElementById('tt');
var ttbg=document.getElementById('ttbg');
var tttl=document.getElementById('tttl');
var ttbd=document.getElementById('ttbd');
var allPG=svgEl.querySelectorAll('.pg');
var activeIdx=-1;

// onEnter is called when the cursor enters a project group.
// It dims all other groups, shows the tooltip, and marks the project active.
function onEnter(idx,evt){
  activeIdx=idx;
  var p=PROJECTS[idx];
  tttl.textContent=p.name;

  // Clear old tooltip rows.
  while(ttbd.firstChild)ttbd.removeChild(ttbd.firstChild);

  // Build one row per snapshot, newest first (points array is oldest-first).
  var y=16;
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

  // Resize tooltip background to fit content.
  var w=220, h=26+y;
  ttbg.setAttribute('width',w);
  ttbg.setAttribute('height',h);

  // Highlight hovered group; dim all others.
  for(var i=0;i<allPG.length;i++){
    allPG[i].style.opacity=(i===idx)?'1':'0.08';
  }
  // Bold the hovered line. allPG[idx] is already that element — no getElementById needed.
  allPG[idx].querySelector('.pl').style.strokeWidth='3';

  moveTT(evt);
  tt.style.display='block';
}

// onLeave restores all groups to their default opacity and hides the tooltip.
function onLeave(){
  tt.style.display='none';
  for(var i=0;i<allPG.length;i++){
    allPG[i].style.opacity='0.55';
    allPG[i].querySelector('.pl').style.strokeWidth='';
  }
  activeIdx=-1;
}

// Follow the cursor while hovering.
svgEl.addEventListener('mousemove',function(evt){
  if(activeIdx>=0)moveTT(evt);
});

// moveTT repositions the tooltip near the cursor, keeping it inside the viewBox.
function moveTT(evt){
  var pt=svgEl.createSVGPoint();
  pt.x=evt.clientX; pt.y=evt.clientY;
  var sp=pt.matrixTransform(svgEl.getScreenCTM().inverse());
  var w=parseFloat(ttbg.getAttribute('width'));
  var h=parseFloat(ttbg.getAttribute('height'));
  var tx=sp.x+14, ty=sp.y-10;
  var vw=svgEl.viewBox.baseVal.width, vh=svgEl.viewBox.baseVal.height;
  if(tx+w>vw-5)tx=sp.x-w-14;
  if(ty+h>vh-5)ty=vh-h-5;
  if(ty<5)ty=5;
  tt.setAttribute('transform','translate('+tx+','+ty+')');
}
]]></script>`, string(projectsJSON))

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
