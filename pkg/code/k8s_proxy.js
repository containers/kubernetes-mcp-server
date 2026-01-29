// Workaround for Goja bug: embedded structs with json tags don't map correctly.
//
// Background: Goja's ExportTo handles anonymous (embedded) fields by passing the
// entire JS object recursively. This works for truly anonymous fields (no json tag
// or json:",inline"). However, when an embedded field has a json tag like
// `json:"metadata"`, Goja should look up that property but doesn't - it still
// passes the entire object due to checking field.Anonymous before considering
// the mapped field name.
//
// Kubernetes Impact: All Kubernetes resources embed ObjectMeta with `json:"metadata"`.
// When JS passes {metadata: {name: "x"}}, Goja fails to extract the "metadata"
// property and tries to map the entire object to ObjectMeta, resulting in empty
// metadata.
//
// Solution: Flatten metadata fields to the top level before passing to Go.
// Since ObjectMeta fields (name, namespace, labels, annotations, etc.) are at
// the top level, Goja correctly populates the embedded struct. Go's JSON
// serialization then restores the nested structure when sending to the API.
//
// Coverage: This workaround covers ALL Kubernetes resources including CRDs because:
// - TypeMeta uses `json:",inline"` which works correctly (no mapping needed)
// - ObjectMeta uses `json:"metadata"` which we handle here
// - All other fields (Spec, Status) are named fields, not embedded

// Methods that accept Kubernetes resources and need conversion
const __mutationMethods = ['create', 'update', 'patch', 'apply', 'Create', 'Update', 'Patch', 'Apply'];

// Methods that return Kubernetes objects and need conversion to pure JS
const __readMethods = ['get', 'list', 'watch', 'Get', 'List', 'Watch'];

// Methods that return another resource interface (for dynamic client chaining)
const __chainingMethods = ['namespace', 'Namespace'];

function __flattenK8sObject(obj) {
	if (!obj || typeof obj !== 'object') {
		return obj;
	}
	if (Array.isArray(obj)) {
		return obj.map(__flattenK8sObject);
	}

	const result = {};
	for (const key in obj) {
		if (key === 'metadata' && typeof obj[key] === 'object') {
			// Flatten metadata fields to top level
			const metadata = obj[key];
			for (const metaKey in metadata) {
				result[metaKey] = __flattenK8sObject(metadata[metaKey]);
			}
		} else {
			result[key] = __flattenK8sObject(obj[key]);
		}
	}
	return result;
}

// Get own enumerable property names from an object
function __getKeys(obj) {
	const keys = [];
	for (const key in obj) {
		keys.push(key);
	}
	return keys;
}

// Try to resolve a property with case-insensitive fallback.
// This allows models to use CoreV1(), coreV1(), COREV1(), Pods(), pods(), PODS(), etc.
function __resolveProperty(obj, prop) {
	// Try exact match first
	if (prop in obj) {
		return obj[prop];
	}
	// Try lowercase version (e.g., CoreV1 -> coreV1)
	const lowerFirst = prop.charAt(0).toLowerCase() + prop.slice(1);
	if (lowerFirst in obj) {
		return obj[lowerFirst];
	}
	// Try uppercase version (e.g., coreV1 -> CoreV1)
	const upperFirst = prop.charAt(0).toUpperCase() + prop.slice(1);
	if (upperFirst in obj) {
		return obj[upperFirst];
	}
	// Try fully lowercase (e.g., List -> list, COREV1 -> corev1)
	const lower = prop.toLowerCase();
	if (lower in obj) {
		return obj[lower];
	}
	// Try case-insensitive search through all keys (handles COREV1 -> coreV1)
	const lowerProp = prop.toLowerCase();
	for (const key in obj) {
		if (key.toLowerCase() === lowerProp) {
			return obj[key];
		}
	}
	return undefined;
}

// Convert Go objects to pure JavaScript objects.
// This is necessary because Go objects with custom types (like Quantity) don't
// work correctly with JavaScript operations. Uses Go's JSON marshaling which
// correctly handles MarshalJSON implementations.
function __toJS(obj) {
	if (obj === null || obj === undefined) {
		return obj;
	}
	// Use the Go-provided JSON conversion function
	// The Go function returns (string, error), and Goja throws on error
	if (typeof __goToJSON === 'function') {
		const jsonStr = __goToJSON(obj);
		const result = JSON.parse(jsonStr);
		// Fix nil slices: Go marshals nil []T as null, but we need [] for JavaScript
		// Common Kubernetes list fields that should be arrays
		if (result && typeof result === 'object') {
			if (result.items === null) {
				result.items = [];
			}
		}
		return result;
	}
	return obj;
}

// Wrap a resource interface (e.g., pods(namespace)) to intercept methods.
// - Mutation methods: flatten metadata on input
// - Read methods: convert Go objects to pure JS on output
// - Chaining methods (namespace): wrap returned interface recursively
// Uses Proxy on an empty object to avoid Go native object invariant issues.
function __wrapResource(resource) {
	return new Proxy({}, {
		get(_, prop) {
			const value = __resolveProperty(resource, prop);
			if (typeof value === 'function') {
				const lowerProp = prop.toLowerCase();
				// Intercept mutation methods to flatten metadata
				if (__mutationMethods.map(function(m) { return m.toLowerCase(); }).indexOf(lowerProp) !== -1) {
					return function() {
						const args = [];
						for (let i = 0; i < arguments.length; i++) {
							const arg = arguments[i];
							// Skip ctx (first arg), flatten resources with metadata
							if (i > 0 && arg && typeof arg === 'object' && arg.metadata) {
								args.push(__flattenK8sObject(arg));
							} else {
								args.push(arg);
							}
						}
						const result = value.apply(resource, args);
						// Also convert mutation results to pure JS
						return __toJS(result);
					};
				}
				// Intercept read methods to convert output to pure JS
				if (__readMethods.map(function(m) { return m.toLowerCase(); }).indexOf(lowerProp) !== -1) {
					return function() {
						const result = value.apply(resource, arguments);
						return __toJS(result);
					};
				}
				// Intercept chaining methods (namespace) to wrap returned interface
				// This is needed for dynamic client: resource(gvr).namespace(ns).list(...)
				if (__chainingMethods.map(function(m) { return m.toLowerCase(); }).indexOf(lowerProp) !== -1) {
					return function() {
						const result = value.apply(resource, arguments);
						return __wrapResource(result);
					};
				}
				// Pass through other methods unchanged
				return function() {
					return value.apply(resource, arguments);
				};
			}
			return value;
		},
		ownKeys(_) {
			return __getKeys(resource);
		},
		getOwnPropertyDescriptor(_, prop) {
			if (__resolveProperty(resource, prop) !== undefined) {
				return { configurable: true, enumerable: true, value: __resolveProperty(resource, prop) };
			}
			return undefined;
		}
	});
}

// Wrap a dynamic resource interface (from dynamicClient().resource(gvr)).
// Unlike typed resources, dynamic resources use *unstructured.Unstructured which preserves
// the nested metadata structure. We use __toUnstructured instead of __flattenK8sObject.
function __wrapDynamicResource(resource) {
	return new Proxy({}, {
		get(_, prop) {
			const value = __resolveProperty(resource, prop);
			if (typeof value === 'function') {
				const lowerProp = prop.toLowerCase();
				// Intercept mutation methods to convert to unstructured
				if (__mutationMethods.map(function(m) { return m.toLowerCase(); }).indexOf(lowerProp) !== -1) {
					return function() {
						const args = [];
						for (let i = 0; i < arguments.length; i++) {
							const arg = arguments[i];
							// Skip ctx (first arg), convert resources with metadata to unstructured
							if (i > 0 && arg && typeof arg === 'object' && arg.metadata && typeof __toUnstructured === 'function') {
								args.push(__toUnstructured(arg));
							} else {
								args.push(arg);
							}
						}
						const result = value.apply(resource, args);
						return __toJS(result);
					};
				}
				// Intercept read methods to convert output to pure JS
				if (__readMethods.map(function(m) { return m.toLowerCase(); }).indexOf(lowerProp) !== -1) {
					return function() {
						const result = value.apply(resource, arguments);
						return __toJS(result);
					};
				}
				// Intercept chaining methods (namespace) to wrap returned interface
				if (__chainingMethods.map(function(m) { return m.toLowerCase(); }).indexOf(lowerProp) !== -1) {
					return function() {
						const result = value.apply(resource, arguments);
						return __wrapDynamicResource(result);
					};
				}
				// Pass through other methods unchanged
				return function() {
					return value.apply(resource, arguments);
				};
			}
			return value;
		},
		ownKeys(_) {
			return __getKeys(resource);
		},
		getOwnPropertyDescriptor(_, prop) {
			if (__resolveProperty(resource, prop) !== undefined) {
				return { configurable: true, enumerable: true, value: __resolveProperty(resource, prop) };
			}
			return undefined;
		}
	});
}

// Wrap a client interface (e.g., coreV1()) to wrap resource accessors.
// Also handles dynamicClient's resource(gvr) method specially since GVR needs conversion.
function __wrapClientInterface(client) {
	return new Proxy({}, {
		get(_, prop) {
			const value = __resolveProperty(client, prop);
			if (typeof value === 'function') {
				const lowerProp = prop.toLowerCase();
				// Special handling for dynamicClient().resource(gvr)
				// The GVR object needs to be converted using __gvr because
				// schema.GroupVersionResource has no json tags.
				// Use __wrapDynamicResource for proper unstructured handling.
				if (lowerProp === 'resource' && typeof __gvr === 'function') {
					return function(gvrObj) {
						// Convert JS object {group, version, resource} to Go GVR struct
						const group = gvrObj.group || gvrObj.Group || '';
						const version = gvrObj.version || gvrObj.Version || '';
						const resource = gvrObj.resource || gvrObj.Resource || '';
						const gvr = __gvr(group, version, resource);
						const result = value.call(client, gvr);
						return __wrapDynamicResource(result);
					};
				}
				// Resource accessor methods (pods, deployments, etc.)
				return function(...args) {
					const resource = value.apply(client, args);
					return __wrapResource(resource);
				};
			}
			return value;
		},
		ownKeys(_) {
			return __getKeys(client);
		},
		getOwnPropertyDescriptor(_, prop) {
			if (__resolveProperty(client, prop) !== undefined) {
				return { configurable: true, enumerable: true, value: __resolveProperty(client, prop) };
			}
			return undefined;
		}
	});
}

// Wrap the main k8s client to wrap all API group accessors.
function __wrapK8sClient(rawK8s) {
	return new Proxy({}, {
		get(_, prop) {
			const value = __resolveProperty(rawK8s, prop);
			if (typeof value === 'function') {
				// API group accessor methods (coreV1, appsV1, etc.)
				return function(...args) {
					const client = value.apply(rawK8s, args);
					return __wrapClientInterface(client);
				};
			}
			return value;
		},
		ownKeys(_) {
			return __getKeys(rawK8s);
		},
		getOwnPropertyDescriptor(_, prop) {
			if (__resolveProperty(rawK8s, prop) !== undefined) {
				return { configurable: true, enumerable: true, value: __resolveProperty(rawK8s, prop) };
			}
			return undefined;
		}
	});
}
