// nulo codemode prelude. Mirrors the globals Pi/PiG give a script, adapted to
// nulo's synchronous host bridge (`nulo_call` returns a string immediately).
//
// The wrapper calls __nulo_init(...) before the script and __nulo_succeed /
// __nulo_fail after it.
(function () {
	"use strict";

	var stringify = JSON.stringify;
	var parse = JSON.parse;
	var ErrorCtor = Error;
	var TypeErrorCtor = TypeError;
	var RangeErrorCtor = RangeError;

	var finished = false;
	var EXIT = Object.freeze({ __nulo_exit: true });

	// Output limits: past them the script fails, so catching the error cannot
	// resume output.
	var OUTPUT_MAX_CHARS = 16777216;
	var OUTPUT_MAX_ITEMS = 100000;
	var outputChars = 0;
	var outputItems = 0;

	function done(ok, payload) {
		if (finished) return;
		finished = true;
		nulo_call("done", ok ? "1" : "0", payload || "");
	}

	function errorText(error) {
		var head = error && error.message ? String(error.name) + ": " + String(error.message) : String(error && error.name ? error.name : error);
		var frames = error && typeof error.stack === "string"
			? error.stack.split("\n").filter(function (line) { return line.trim() && line.indexOf("codemode-prelude.js") < 0; })
			: [];
		return [head].concat(frames).join("\n");
	}

	// JSON so the host gets stable string fields.
	function describeError(error) {
		try {
			if (error instanceof ErrorCtor) {
				return stringify({ name: String(error.name), message: String(error.message), stack: errorText(error) });
			}
			return stringify({ message: format(error) });
		} catch (e) {
			return stringify({ message: "The script threw a value that cannot be described" });
		}
	}

	// Like Pi's `format`: strings as is, Errors as "Name: message", everything
	// else compact JSON.
	function format(value) {
		if (typeof value === "string") return value;
		if (value instanceof ErrorCtor) return errorText(value);
		try {
			var json = stringify(value);
			return json === undefined ? String(value) : json;
		} catch (e) {
			return String(value);
		}
	}

	// Like the script's text(): primitives become their string form, everything
	// else JSON. Errors keep their name and message.
	function outputText(value) {
		if (value === undefined || value === null || typeof value !== "object" && typeof value !== "function") {
			return String(value);
		}
		if (value instanceof ErrorCtor) {
			return stringify({ name: String(value.name), message: String(value.message) });
		}
		var json = stringify(value);
		return json === undefined ? String(value) : json;
	}

	function output(kind, data, mimeType) {
		if (finished) return;
		outputChars += data.length;
		outputItems++;
		if (outputChars > OUTPUT_MAX_CHARS || outputItems > OUTPUT_MAX_ITEMS) {
			var error = new RangeErrorCtor(
				"script output exceeded the limit of 16777216 characters or 100000 text(), image(), and console calls. " +
				"Print a summary instead, or write large data to a file with a tool.");
			done(false, describeError(error));
			throw error;
		}
		nulo_call("output", kind, data, mimeType || "");
	}

	function text(value) {
		var rendered;
		try {
			rendered = outputText(value);
		} catch (error) {
			throw new TypeErrorCtor(error instanceof ErrorCtor ? error.message : String(error));
		}
		output("text", rendered);
	}

	function imageUrl(value) {
		var wrong = "image expects a non-empty image URL string, an object with image_url, or an image block { type: \"image\", data, mimeType }";
		if (typeof value === "string") return value;
		if (typeof value !== "object" || value === null || Array.isArray(value)) throw new TypeErrorCtor(wrong);
		if (value.image_url !== undefined) {
			if (typeof value.image_url !== "string") throw new TypeErrorCtor(wrong);
			return value.image_url;
		}
		if (typeof value.type !== "string") throw new TypeErrorCtor(wrong);
		if (value.type !== "image") throw new TypeErrorCtor('image only accepts image blocks, got "' + value.type + '"');
		if (typeof value.data !== "string" || value.data === "") throw new TypeErrorCtor("image expected image block data");
		if (value.data.toLowerCase().indexOf("data:") === 0) return value.data;
		return "data:;base64," + value.data;
	}

	// Providers accept PNG, JPEG (not JPEG-LS), GIF and WebP inline. Signatures
	// start at byte 0, so their base64 encodings are prefixes.
	var IMAGE_SIGNATURES = [
		["image/png", /^iVBORw0KGg/],
		["image/jpeg", /^[/]9j[/](?!9)/],
		["image/gif", /^R0lGOD[dl]h/],
		["image/webp", /^UklG.{8}RUJQ/]
	];

	function image(value) {
		var url = imageUrl(value);
		if (url === "") throw new TypeErrorCtor("image expects a non-empty image URL string, an object with image_url, or an image block { type: \"image\", data, mimeType }");
		var colon = url.indexOf(":");
		var scheme = colon === -1 ? "" : url.slice(0, colon).toLowerCase();
		if (scheme === "http" || scheme === "https") {
			throw new TypeErrorCtor("remote image URLs are not supported in tool outputs. Pass a base64 data URI instead");
		}
		var comma = url.indexOf(",");
		var header = comma === -1 ? [] : url.slice(colon + 1, comma).split(";");
		if (scheme !== "data" || comma === -1 || !header.slice(1).some(function (part) { return part.toLowerCase() === "base64"; })) {
			throw new TypeErrorCtor("invalid image output. Pass a base64 data URI instead");
		}
		// Line breaks from wrapped base64 are dropped; the detected type wins.
		var data = url.slice(comma + 1).replace(/\s+/g, "");
		if (data.length % 4 !== 0 || !/^[A-Za-z0-9+/]+={0,2}$/.test(data)) {
			throw new TypeErrorCtor("invalid image output. The image data is not valid base64 (truncated or corrupted?)");
		}
		var head = data.slice(0, 16);
		var signature = null;
		for (var i = 0; i < IMAGE_SIGNATURES.length; i++) {
			if (IMAGE_SIGNATURES[i][1].test(head)) { signature = IMAGE_SIGNATURES[i]; break; }
		}
		if (!signature) throw new TypeErrorCtor("invalid image output. The image data is not a PNG, JPEG, GIF, or WebP image");
		output("image", data, signature[0]);
	}

	// store()/load(): key -> JSON text. Limits match Pi: one value may hold 262144
	// characters of JSON and the whole store 1048576.
	var stored = new Map();
	var writes = new Map();
	var storedChars = 0;
	var STORE_HINT = "store() is for small state such as IDs or summaries. Show images with image(), keep large data in variables, or write it to a file with a tool.";

	function checkKey(name, key) {
		if (typeof key !== "string") throw new TypeErrorCtor(name + "() key must be a string");
	}

	function store(key, value) {
		checkKey("store", key);
		var previous = stored.has(key) ? key.length + stored.get(key).length : 0;
		if (value === undefined) {
			stored.delete(key);
			storedChars -= previous;
			writes.set(key, undefined);
			return;
		}
		var json;
		try {
			json = stringify(value);
		} catch (error) {
			throw new TypeErrorCtor("store(" + stringify(key) + ") value is not JSON-serializable: " + format(error));
		}
		if (json === undefined) throw new TypeErrorCtor("store(" + stringify(key) + ") value is not JSON-serializable");
		if (json.length > 262144) {
			throw new RangeErrorCtor("store(" + stringify(key) + ") value has " + json.length + " characters of JSON, more than the limit of 262144. " + STORE_HINT);
		}
		var next = storedChars - previous + key.length + json.length;
		if (next > 1048576) {
			throw new RangeErrorCtor("store is full: stored values would exceed 1048576 characters of JSON. Delete keys with store(key, undefined). " + STORE_HINT);
		}
		stored.set(key, json);
		storedChars = next;
		writes.set(key, json);
	}

	function load(key) {
		checkKey("load", key);
		var json = stored.get(key);
		return json === undefined ? undefined : parse(json);
	}

	function serializeWrites() {
		var entries = [];
		writes.forEach(function (json, key) { entries.push(json === undefined ? [key] : [key, json]); });
		return stringify(entries);
	}

	// Reading a member that does not exist names the close matches instead of
	// failing later with "not a function".
	function comparable(name) { return name.toLowerCase().replace(/[^a-z0-9]/g, ""); }

	function guard(target, label, names, hint) {
		return new Proxy(target, {
			get: function (object, property, receiver) {
				if (typeof property !== "string" || property in object || property in Object.prototype || property === "then" || property === "toJSON") {
					return Reflect.get(object, property, receiver);
				}
				var wanted = comparable(property);
				var exact = names.filter(function (name) { return comparable(name) === wanted; });
				var close = exact.length > 0 ? exact : names.filter(function (name) {
					return wanted && (comparable(name).indexOf(wanted) >= 0 || wanted.indexOf(comparable(name)) >= 0);
				});
				var message = label + "." + property + " does not exist.";
				if (close.length > 0) message += " Did you mean " + close.slice(0, 5).map(function (name) { return label + "." + name; }).join(", ") + "?";
				else if (names.length <= 20) message += " Available: " + names.join(", ") + ".";
				if (hint) message += " " + hint;
				message += ' Check for a member with "' + property + '" in ' + label + ".";
				throw new TypeErrorCtor(message);
			}
		});
	}

	function exit() {
		var writesJson = "[]";
		try {
			writesJson = serializeWrites();
		} catch (error) {
			done(false, describeError(error));
			throw EXIT;
		}
		nulo_call("writes", writesJson);
		done(true, "");
		throw EXIT;
	}

	// __nulo_init installs the per-run globals.
	globalThis.__nulo_init = function (toolsJson, storeJson, namespacesJson) {
		var tools = Object.create(null);
		var allTools = [];
		var byName = Object.create(null);
		parse(toolsJson).forEach(function (entry) {
			var fn = function (args) {
				var result = parse(nulo_call("call", entry.call, stringify(args === undefined ? {} : args)));
				if (result.error) throw new ErrorCtor(result.error);
				return result.ok;
			};
			var publicEntry = Object.freeze({ name: entry.name, description: entry.description });
			if (!(entry.name in byName)) {
				tools[entry.name] = fn;
				allTools.push(publicEntry);
			}
			byName[entry.name] = entry;
			byName[entry.call] = entry;
		});
		Object.freeze(allTools);

		var storeTable = parse(storeJson || "{}");
		Object.keys(storeTable).forEach(function (key) {
			var json = stringify(storeTable[key]);
			stored.set(key, json);
			storedChars += key.length + json.length;
		});

		var namespaces = Object.create(null);
		var nsTable = parse(namespacesJson || "{}");
		Object.keys(nsTable).forEach(function (key) { namespaces[key] = nsTable[key]; });

		function normNS(s) {
			s = String(s === undefined || s === null ? "" : s).toLowerCase().replace(/^\s+|\s+$/g, "");
			if (s.indexOf("mcp__") === 0) s = s.slice(5);
			return s.replace(/-/g, "_");
		}

		// Ranking happens in Go so codemode and tool_search share one BM25.
		globalThis.searchTools = function (query, options) {
			options = options || {};
			var payload = stringify({
				query: query === undefined || query === null ? "" : String(query),
				limit: options.limit === undefined ? null : options.limit,
				namespace: options.namespace === undefined ? null : String(options.namespace)
			});
			return parse(nulo_call("search", payload));
		};
		globalThis.describeTool = function (name) {
			var entry = byName[String(name)];
			return entry === undefined ? undefined : entry.description;
		};
		globalThis.describeNamespace = function (name) {
			var value = namespaces[normNS(name)];
			return value === undefined ? undefined : value;
		};

		globalThis.tools = guard(tools, "tools", allTools.map(function (tool) { return tool.name; }),
			"ALL_TOOLS lists every tool; searchTools(query) finds tools by topic.");
		globalThis.ALL_TOOLS = allTools;

		var consoleObject = {};
		["log", "info", "warn", "error", "debug"].forEach(function (level) {
			consoleObject[level] = function () {
				output("console", Array.prototype.map.call(arguments, format).join(" "));
			};
		});
		globalThis.console = consoleObject;
		globalThis.text = text;
		globalThis.image = image;
		globalThis.exit = exit;
		globalThis.store = store;
		globalThis.load = load;
	};

	globalThis.__nulo_succeed = function (value) {
		if (finished) return;
		if (value !== undefined) {
			try {
				text(value);
			} catch (error) {
				done(false, describeError(error));
				return;
			}
		}
		var writesJson = "[]";
		try {
			writesJson = serializeWrites();
		} catch (error) {
			done(false, describeError(error));
			return;
		}
		nulo_call("writes", writesJson);
		done(true, "");
	};

	globalThis.__nulo_fail = function (error) {
		done(false, describeError(error));
	};
})();
