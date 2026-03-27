package graph

import (
	"encoding/json"
	"fmt"
	"strings"
)

func GenerateHTML(data *GraphData, title string) (string, error) {
	nodesJSON, err := json.Marshal(data.Nodes)
	if err != nil {
		return "", fmt.Errorf("marshaling nodes: %w", err)
	}
	edgesJSON, err := json.Marshal(data.Edges)
	if err != nil {
		return "", fmt.Errorf("marshaling edges: %w", err)
	}

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>`)
	b.WriteString(title)
	b.WriteString(`</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { background: #1a1b26; color: #c0caf5; font-family: -apple-system, system-ui, sans-serif; overflow: hidden; }
  svg { width: 100vw; height: 100vh; display: block; }
  .node circle { stroke: #414868; stroke-width: 1.5px; cursor: pointer; }
  .node text { font-size: 11px; fill: #c0caf5; pointer-events: none; }
  .link { stroke: #414868; stroke-opacity: 0.6; }
  .node circle:hover { stroke: #7aa2f7; stroke-width: 2px; }
  #info { position: fixed; top: 16px; right: 16px; background: #24283b; padding: 12px 16px; border-radius: 8px; font-size: 13px; line-height: 1.6; border: 1px solid #414868; }
  #info span { color: #7aa2f7; font-weight: 600; }
</style>
</head>
<body>
<div id="info">
  <span id="stat-nodes">0</span> notes &middot;
  <span id="stat-edges">0</span> links &middot;
  <span id="stat-orphans">0</span> orphans
</div>
<svg id="graph"></svg>
<script src="https://d3js.org/d3.v7.min.js"></script>
<script>
const nodes = `)
	b.Write(nodesJSON)
	b.WriteString(`;
const links = `)
	b.Write(edgesJSON)
	b.WriteString(`;

document.getElementById("stat-nodes").textContent = nodes.length;
document.getElementById("stat-edges").textContent = links.length;
document.getElementById("stat-orphans").textContent = nodes.filter(n => n.connections === 0).length;

const svg = d3.select("#graph");
const width = window.innerWidth;
const height = window.innerHeight;

const simulation = d3.forceSimulation(nodes)
  .force("link", d3.forceLink(links).id(d => d.id).distance(120))
  .force("charge", d3.forceManyBody().strength(-300))
  .force("center", d3.forceCenter(width / 2, height / 2))
  .force("collision", d3.forceCollide().radius(d => nodeRadius(d) + 8));

const g = svg.append("g");

svg.call(d3.zoom().scaleExtent([0.1, 4]).on("zoom", e => g.attr("transform", e.transform)));

const link = g.append("g").selectAll("line")
  .data(links).enter().append("line").attr("class", "link");

const node = g.append("g").selectAll("g")
  .data(nodes).enter().append("g").attr("class", "node")
  .call(d3.drag()
    .on("start", (e, d) => { if (!e.active) simulation.alphaTarget(0.3).restart(); d.fx = d.x; d.fy = d.y; })
    .on("drag", (e, d) => { d.fx = e.x; d.fy = e.y; })
    .on("end", (e, d) => { if (!e.active) simulation.alphaTarget(0); d.fx = null; d.fy = null; }));

function nodeRadius(d) { return Math.max(6, Math.min(20, 4 + d.connections * 3)); }
function nodeColor(d) {
  if (!d.workspace_id) return "#7aa2f7";
  const h = Array.from(d.workspace_id).reduce((a, c) => a + c.charCodeAt(0), 0) % 360;
  return "hsl(" + h + ", 60%, 65%)";
}

node.append("circle").attr("r", nodeRadius).attr("fill", nodeColor);
node.append("text").text(d => d.label).attr("dx", d => nodeRadius(d) + 4).attr("dy", 4);

node.append("title").text(d => d.label + " (" + d.connections + " connections)");

simulation.on("tick", () => {
  link.attr("x1", d => d.source.x).attr("y1", d => d.source.y)
      .attr("x2", d => d.target.x).attr("y2", d => d.target.y);
  node.attr("transform", d => "translate(" + d.x + "," + d.y + ")");
});
</script>
</body>
</html>`)

	return b.String(), nil
}
