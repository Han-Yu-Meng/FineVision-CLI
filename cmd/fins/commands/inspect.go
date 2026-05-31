package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"fins-cli/cmd/fins/client"
	"fins-cli/internal/core"
	"fins-cli/internal/utils"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type InspectResult struct {
	Nodes []Node `json:"nodes"`
}

type Node struct {
	Name        string      `json:"name"`
	Category    string      `json:"category"`
	Description string      `json:"description"`
	Inputs      []Port      `json:"inputs"`
	Outputs     []Port      `json:"outputs"`
	Parameters  []Parameter `json:"parameters"`
	PackageName string      `json:"package_name"`
}

type Port struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Parameter struct {
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	DefaultValue interface{} `json:"default_value"`
}

var (
	inspectFile string
)

type model struct {
	nodes      []Node
	cursor     int
	startIndex int // Index of the first node to display
	height     int // Maximum number of nodes to display at once
	width      int // Terminal width
	maxNameLen int // Length of the longest node name for dynamic width
	pkgName    string
	copied     bool
	quitting   bool
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		// If height is not set or we want it dynamic:
		// m.height = msg.Height - offset
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.copied = false
				// Scroll up if cursor moves above startIndex
				if m.cursor < m.startIndex {
					m.startIndex = m.cursor
				}
			}
		case "down", "j":
			if m.cursor < len(m.nodes)-1 {
				m.cursor++
				m.copied = false
				// Scroll down if cursor moves below visible area
				if m.cursor >= m.startIndex+m.height {
					m.startIndex = m.cursor - m.height + 1
				}
			}
		case "enter":
			node := m.nodes[m.cursor]
			snippet := generateSnippet(node)
			clipboard.WriteAll(snippet)
			m.copied = true
		}
	}
	return m, nil
}

func generateSnippet(n Node) string {
	var sb strings.Builder
	sb.WriteString("Node(\n")
	sb.WriteString(fmt.Sprintf("    package=\"%s\",\n", n.PackageName))
	sb.WriteString(fmt.Sprintf("    name=\"%s\",\n", n.Name))

	if len(n.Inputs) > 0 {
		sb.WriteString("    inputs={\n")
		for _, in := range n.Inputs {
			sb.WriteString(fmt.Sprintf("        \"%s\": \"\",\n", in.Name))
		}
		sb.WriteString("    },\n")
	}

	if len(n.Outputs) > 0 {
		sb.WriteString("    outputs={\n")
		for _, out := range n.Outputs {
			sb.WriteString(fmt.Sprintf("        \"%s\": \"\",\n", out.Name))
		}
		sb.WriteString("    },\n")
	}

	if len(n.Parameters) > 0 {
		sb.WriteString("    parameters={\n")
		for _, p := range n.Parameters {
			val := p.DefaultValue
			if val == nil || val == "" {
				val = "\"\""
			} else if s, ok := val.(string); ok {
				val = fmt.Sprintf("\"%s\"", s)
			}
			sb.WriteString(fmt.Sprintf("        \"%s\": %v,\n", p.Name, val))
		}
		sb.WriteString("    },\n")
	}

	sb.WriteString("),")
	return sb.String()
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262"))

	typeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#777777")).
			Italic(true)

	labelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00ADD8"))

	portNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00ADD8"))

	paramNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00ADD8"))

	borderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#444444"))
)

func (m model) View() string {
	if m.quitting {
		return ""
	}

	// Use a default width if not yet received from WindowSizeMsg
	width := m.width
	if width == 0 {
		width = 100
	}

	endIndex := m.startIndex + m.height
	if endIndex > len(m.nodes) {
		endIndex = len(m.nodes)
	}

	header := titleStyle.Render(fmt.Sprintf("Inspect: %s (%d nodes)", m.pkgName, len(m.nodes)))
	scrollInfo := ""
	if len(m.nodes) > m.height {
		scrollInfo = dimStyle.Render(fmt.Sprintf(" [Scroll: %d-%d/%d]", m.startIndex+1, endIndex, len(m.nodes)))
	}
	help := dimStyle.Render("")
	if m.copied {
		help = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("          [Copied to clipboard!]")
	}

	topBar := header + scrollInfo + help + "\n"
	separator := borderStyle.Render(strings.Repeat("-", width)) + "\n"

	var leftPane strings.Builder
	for i := m.startIndex; i < endIndex; i++ {
		node := m.nodes[i]
		cursor := "  "
		style := lipgloss.NewStyle()
		if m.cursor == i {
			cursor = selectedStyle.Render("> ")
			style = selectedStyle
		} else {
			style = style.Foreground(lipgloss.Color("#AAAAAA"))
		}

		// Show arrows for scrolling
		prefix := "  "
		if i == m.startIndex && m.startIndex > 0 {
			prefix = "↑ "
		} else if i == endIndex-1 && endIndex < len(m.nodes) {
			prefix = "↓ "
		}

		line := fmt.Sprintf("%s%s%s", prefix, cursor, style.Render(node.Name))
		// Pad the line to a dynamic width using lipgloss
		paddedLine := lipgloss.NewStyle().Width(m.maxNameLen).Render(line)
		leftPane.WriteString(paddedLine + "\n")
	}

	for i := endIndex - m.startIndex; i < m.height; i++ {
		leftPane.WriteString(lipgloss.NewStyle().Width(m.maxNameLen).Render("") + "\n")
	}

	selectedNode := m.nodes[m.cursor]
	rightPaneWidth := width - m.maxNameLen - 5 // 5 for margins and separator

	if rightPaneWidth < 20 {
		rightPaneWidth = 20
	}

	var rightPane strings.Builder

	renderField := func(label, value string) {
		labelRendered := labelStyle.Render(label)
		// Calculate available width for the value
		valWidth := rightPaneWidth - lipgloss.Width(labelRendered) - 1
		if valWidth < 10 {
			valWidth = 10
		}
		valRendered := lipgloss.NewStyle().Width(valWidth).Render(value)
		
		// Split value into lines to handle indentation of subsequent lines
		valLines := strings.Split(valRendered, "\n")
		for i, line := range valLines {
			if i == 0 {
				rightPane.WriteString(fmt.Sprintf("  %s %s\n", labelRendered, line))
			} else {
				// Indent subsequent lines by the width of the label + spaces
				indent := strings.Repeat(" ", lipgloss.Width(labelRendered)+3)
				rightPane.WriteString(fmt.Sprintf("%s%s\n", indent, line))
			}
		}
	}

	renderField("Node:", selectedNode.Name)
	renderField("Category:", selectedNode.Category)
	renderField("Description:", selectedNode.Description)

	if len(selectedNode.Inputs) > 0 {
		rightPane.WriteString(fmt.Sprintf("\n  %s\n", labelStyle.Render("Inputs:")))
		for _, in := range selectedNode.Inputs {
			rightPane.WriteString(fmt.Sprintf("    %s %s\n", portNameStyle.Render(in.Name), typeStyle.Render(in.Type)))
		}
	}

	if len(selectedNode.Outputs) > 0 {
		rightPane.WriteString(fmt.Sprintf("\n  %s\n", labelStyle.Render("Outputs:")))
		for _, out := range selectedNode.Outputs {
			rightPane.WriteString(fmt.Sprintf("    %s %s\n", portNameStyle.Render(out.Name), typeStyle.Render(out.Type)))
		}
	}

	if len(selectedNode.Parameters) > 0 {
		rightPane.WriteString(fmt.Sprintf("\n  %s\n", labelStyle.Render("Parameters:")))
		for _, p := range selectedNode.Parameters {
			rightPane.WriteString(fmt.Sprintf("    %s %s\n", paramNameStyle.Render(p.Name), typeStyle.Render(fmt.Sprintf("%s", p.Type))))
		}
	}

	leftLines := strings.Split(strings.TrimSuffix(leftPane.String(), "\n"), "\n")
	rightLines := strings.Split(strings.TrimSuffix(rightPane.String(), "\n"), "\n")

	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}

	var body strings.Builder
	for i := 0; i < maxLines; i++ {
		left := lipgloss.NewStyle().Width(m.maxNameLen).Render("")
		if i < len(leftLines) {
			left = leftLines[i]
		}
		// Ensure left is always maxNameLen wide even if it was shorter
		left = lipgloss.NewStyle().Width(m.maxNameLen).Render(left)

		right := ""
		if i < len(rightLines) {
			right = rightLines[i]
		}
		body.WriteString(fmt.Sprintf("%s %s %s\n", left, borderStyle.Render("|"), right))
	}

	return topBar + separator + body.String() + separator
}

var inspectCmd = &cobra.Command{
	Use:   "inspect [package]",
	Short: "Inspect a compiled package binary",
	Long:  `Analyze the shared object (.so) file of a package to reveal its architecture, dependencies, and FINS nodes.`,
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if inspectFile != "" {
			var targetPath string

			if filepath.IsAbs(inspectFile) {
				targetPath = inspectFile
			} else {
				matches, _ := filepath.Glob(inspectFile)
				if len(matches) == 1 {
					targetPath = matches[0]
				} else if len(matches) > 1 {
					utils.LogError(os.Stdout, "Ambiguous file pattern: %d files match '%s' in current directory", len(matches), inspectFile)
					return
				} else {
					home, _ := os.UserHomeDir()
					installDir := filepath.Join(home, ".fins/install")
					pattern := filepath.Join(installDir, inspectFile)
					installMatches, _ := filepath.Glob(pattern)

					if len(installMatches) == 0 {
						utils.LogError(os.Stdout, "No file found matching '%s' in current directory or %s", inspectFile, installDir)
						return
					}
					if len(installMatches) > 1 {
						var foundNames []string
						for _, m := range installMatches {
							foundNames = append(foundNames, filepath.Base(m))
						}
						utils.LogError(os.Stdout, "Ambiguous file pattern: %d files match in %s: %v", len(installMatches), installDir, foundNames)
						return
					}
					targetPath = installMatches[0]
				}
			}

			if strings.ContainsAny(targetPath, "*?[]") {
				matches, _ := filepath.Glob(targetPath)
				if len(matches) == 0 {
					utils.LogError(os.Stdout, "No file found matching pattern: %s", targetPath)
					return
				}
				if len(matches) > 1 {
					utils.LogError(os.Stdout, "Ambiguous file pattern: %d files match: %v", len(matches), matches)
					return
				}
				targetPath = matches[0]
			}

			absPath, err := filepath.Abs(targetPath)
			if err != nil {
				utils.LogError(os.Stdout, "Failed to resolve absolute path: %v", err)
				return
			}

			u, _ := url.Parse(fmt.Sprintf("%s/api/inspect/file", DaemonURL))
			q := u.Query()
			q.Set("path", absPath)
			u.RawQuery = q.Encode()

			resp, err := http.Get(u.String())
			if err != nil {
				utils.LogError(os.Stdout, "Error connecting to finsd: %v", err)
				return
			}
			defer resp.Body.Close()

			handleInspectResponse(resp)
			return
		}

		if len(args) == 0 {
			cmd.Help()
			return
		}

		pkgName := args[0]

		if pkgName == "build" {
			inspectBuildCmd.Run(cmd, args)
			return
		}

		finalPkg, err := client.ResolvePackageIdentity(DaemonURL, pkgName, targetSource)
		if err != nil {
			utils.LogError(os.Stdout, "%v", err)

			return
		}

		apiURL := fmt.Sprintf("%s/api/inspect/analyze/%s", DaemonURL, finalPkg)

		resp, err := http.Get(apiURL)
		if err != nil {
			utils.LogError(os.Stdout, "Error connecting to finsd: %v", err)
			return
		}
		defer resp.Body.Close()

		handleInspectResponse(resp)
	},
}

func handleInspectResponse(resp *http.Response) {
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		var errObj map[string]interface{}
		if json.Unmarshal(body, &errObj) == nil {
			if msg, ok := errObj["error"].(string); ok {
				utils.LogError(os.Stdout, "Failed to inspect package: %s", msg)
				return
			}
		}
		utils.LogError(os.Stdout, "Failed to inspect package:")
		fmt.Println(string(body))
		return
	}

	rawJSON, err := io.ReadAll(resp.Body)
	if err != nil {
		utils.LogError(os.Stdout, "Failed to read response: %v", err)
		return
	}

	var results []InspectResult
	if err := json.Unmarshal(rawJSON, &results); err != nil {
		utils.LogError(os.Stdout, "Failed to parse inspect results: %v", err)
		return
	}

	if len(results) == 0 || len(results[0].Nodes) == 0 {
		utils.LogWarning(os.Stdout, "No nodes found in package.")
		return
	}

	pkgName := results[0].Nodes[0].PackageName
	if pkgName == "" {
		pkgName = "Unknown"
	}

	maxLen := 0
	for _, node := range results[0].Nodes {
		if len(node.Name) > maxLen {
			maxLen = len(node.Name)
		}
	}
	// Add padding for cursor "> " and prefix "↑ "
	maxLen += 5
	if maxLen < 20 {
		maxLen = 20
	}

	m := model{
		nodes:      results[0].Nodes,
		pkgName:    pkgName,
		height:     15, // Display 15 nodes at a time
		maxNameLen: maxLen,
	}

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI: %v", err)
		os.Exit(1)
	}
}

var inspectBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the inspect tool binary",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		utils.LogSection(os.Stdout, "Building inspect tool binary")

		err := core.CompileInspect(context.Background(), os.Stdout)
		if err != nil {
			utils.LogError(os.Stdout, "Build failed: %v", err)
			return
		}
		utils.LogSuccess(os.Stdout, "Inspect tool built successfully.")
	},
}

func init() {
	inspectCmd.AddCommand(inspectBuildCmd)
	inspectCmd.Flags().StringVar(&targetSource, "source", "", "Specify package source to resolve ambiguity")
	inspectCmd.Flags().StringVar(&inspectFile, "file", "", "Analyze a specific .so file directly")
	RootCmd.AddCommand(inspectCmd)
}