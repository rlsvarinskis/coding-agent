[SYSTEM: COMMAND PHASE] Provide an action. Your response must be enclosed in an XML tag that calls one of the available tools. Don't forget to close the XML tag at the end, and don't forget to escape any XML characters within the body. Do NOT use the shortened closing form of <tag />, always close it properly.

You can only run one command for now. If you wish to execute multiple commands, run writing or reading notes first. Do NOT write any thoughts before, start immediately with the command.
<example>
<write_file file="/cmd/main.go" start-line="56" end-line="86" >
func check_smaller(x int) {
    return x &lt; 3
}
</write_file>
</example>