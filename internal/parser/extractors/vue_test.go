package extractors

import (
	"strings"
	"testing"
)

func TestExtractVueScriptKeepsScriptDropsTemplate(t *testing.T) {
	sfc := `<template>
  <div>{{ message }}</div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
function greet() { return 'hi' }
const message = ref('hello')
</script>

<style scoped>
div { color: red; }
</style>
`
	out := string(extractVueScript([]byte(sfc)))

	// Script content survives.
	if !strings.Contains(out, "function greet()") {
		t.Errorf("script body lost; got:\n%s", out)
	}
	if !strings.Contains(out, "import { ref }") {
		t.Errorf("import lost; got:\n%s", out)
	}
	// Template/style are gone.
	if strings.Contains(out, "{{ message }}") || strings.Contains(out, "color: red") {
		t.Errorf("template/style leaked into output:\n%s", out)
	}
	// Line numbers preserved: greet() is on line 7 in the SFC and in the output.
	lines := strings.Split(out, "\n")
	if len(lines) < 7 || !strings.Contains(lines[6], "function greet()") {
		t.Errorf("line offset not preserved; line 7 = %q", safeLine(lines, 6))
	}
}

func TestExtractVueScriptMultipleBlocks(t *testing.T) {
	sfc := `<script>
export default { name: 'Foo' }
</script>
<template><p/></template>
<script setup>
function setupFn() {}
</script>
`
	out := string(extractVueScript([]byte(sfc)))
	if !strings.Contains(out, "export default") {
		t.Error("first script block lost")
	}
	if !strings.Contains(out, "function setupFn()") {
		t.Error("second script block lost")
	}
	if strings.Contains(out, "<p/>") {
		t.Error("template leaked")
	}
}

func safeLine(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}
