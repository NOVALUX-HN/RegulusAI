// Memory CLI argv builders (NOV-67 split).

// memoryHistoryArgs builds the `memory history` argv: the single
// topic_key param reaches --topic-key. Pure so the S4 flag-parity pin
// holds without scraping the plugin source (which carries the host
// import and cannot be unit-imported).
export function memoryHistoryArgs(topicKey: string): string[] {
  return ["memory", "history", "--topic-key", topicKey]
}

// memorySaveArgs builds the `memory save` argv. Pre-existing CLI
// convention: the comma-joined inputs keep singular --flags for the
// plural props (--path for paths, --receipt for receipts).
export function memorySaveArgs(input: Record<string, string>): string[] {
  const { type, title, content, topic_key, domain, task, paths, receipts } = input
  const args = ["memory", "save", "--type", type, "--title", title]
  if (topic_key) args.push("--topic-key", topic_key)
  if (domain) args.push("--domain", domain)
  if (task) args.push("--task", task)
  if (paths) args.push("--path", paths)
  if (receipts) args.push("--receipt", receipts)
  args.push(content)
  return args
}

// memorySearchArgs builds the `memory search` argv, mirroring the MCP
// coercion the tool schema advertises: limit is an integer (0 means
// all), any/latest/include_superseded are booleans (flag iff truthy),
// terms split on whitespace.
export function memorySearchArgs(input: Record<string, string | number | boolean>): string[] {
  const { terms, topic_key, domain, dimension, path, limit, any, latest, include_superseded, type } = input
  const args = ["memory", "search"]
  if (topic_key) args.push("--topic-key", String(topic_key))
  if (domain) args.push("--domain", String(domain))
  if (dimension) args.push("--dimension", String(dimension))
  if (path) args.push("--path", String(path))
  if (limit !== undefined && limit !== "" && Number(limit) !== 0) args.push("--limit", String(Number(limit)))
  else if (String(limit) === "0") args.push("--limit", "0")
  const truthy = (v: unknown) => v === true || v === "true"
  if (truthy(any)) args.push("--any")
  if (truthy(latest)) args.push("--latest")
  if (truthy(include_superseded)) args.push("--include-superseded")
  if (type) args.push("--type", String(type))
  for (const term of String(terms ?? "").split(/\s+/).filter((t) => t !== "")) {
    args.push(term)
  }
  return args
}
