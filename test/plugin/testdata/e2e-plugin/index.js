import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import * as grpc from "./vendor/node_modules/@grpc/grpc-js/build/src/index.js";
import * as protoLoader from "./vendor/node_modules/@grpc/proto-loader/build/src/index.js";

const pluginID = "com.example.e2e";
const credential = process.env.GOSPEAK_PLUGIN_CREDENTIAL;
const stateDir = process.env.GOSPEAK_PLUGIN_STATE_DIR;
if (!credential || !stateDir) throw new Error("missing plugin environment");
const endpoint = readFileSync(credential, "utf8").trim();
const pageURL = process.env.GOSPEAK_TEST_PAGE_URL;

const definition = protoLoader.loadSync(join(process.cwd(), "plugin.proto"), {
	keepCase: true,
	longs: String,
	defaults: true,
	oneofs: true,
});
const proto = grpc.loadPackageDefinition(definition).gospeak.plugin.v1;
const registration = new proto.RegistrationService(
	endpoint,
	grpc.credentials.createInsecure(),
);
const api = new proto.PluginAPIService(endpoint, grpc.credentials.createInsecure());
const hookServer = new grpc.Server();

function call(client, method, request) {
	return new Promise((resolve, reject) => {
		client[method](request, (err, response) => {
			if (err) reject(err);
			else resolve(response);
		});
	});
}

async function main() {
	hookServer.addService(proto.PluginHookService.service, {
		HandleEvent: (call, callback) => callback(null, {}),
	});
	const hookPort = await new Promise((resolve, reject) => {
		hookServer.bindAsync("127.0.0.1:0", grpc.ServerCredentials.createInsecure(), (err, port) => {
			if (err) reject(err);
			else resolve(port);
		});
	});
	hookServer.start();
	const pages = pageURL
		? [{ route_path: "/plugins/com.example.e2e/", mode: "iframe", entry_url: pageURL }]
		: [];
	const response = await call(registration, "Register", {
		plugin_id: pluginID,
		name: "E2E fixture plugin",
		version: "1.0.0",
		hook_types: ["message.created"],
		hook_endpoint: "127.0.0.1:" + hookPort,
		pages,
	});
	const calls = [];
	const record = async (label, promise) => {
		try {
			const result = await promise;
			calls.push({ label, ok: true, result });
		} catch (err) {
			calls.push({ label, ok: false, error: String(err?.message ?? err) });
		}
		writeFileSync(join(stateDir, "calls.json"), JSON.stringify(calls, null, 2));
	};
	await record("GetUser", call(api, "GetUser", { user_uuid: "00000000-0000-0000-0000-000000000001" }));
	await record("ListRooms", call(api, "ListRooms", { domain_uuid: "00000000-0000-0000-0000-000000000002" }));
	await record("GetConfig", call(api, "GetConfig", { plugin_id: pluginID }));
	await record("KVSave", call(api, "KVSave", { plugin_id: pluginID, key: "probe", value_json: "{\"v\":1}", reason: "e2e", request_id: "e2e-kv-1" }));
	await record("KVLoad", call(api, "KVLoad", { plugin_id: pluginID, key: "probe" }));
	await record("CreateRoom", call(api, "CreateRoom", { domain_uuid: "00000000-0000-0000-0000-000000000002", name: "plugin-e2e-room", reason: "e2e", request_id: "e2e-room-1" }));
	await record("ReportError", call(api, "ReportError", { plugin_id: pluginID, message: "e2e probe", details_json: "{}", request_id: "e2e-error-1" }));
	setInterval(() => {
		call(registration, "Heartbeat", { plugin_id: pluginID }).catch(() => {});
	}, 5000).unref();
	process.on("SIGTERM", () => {
		hookServer.tryShutdown(() => process.exit(0));
	});
	writeFileSync(join(stateDir, "registered.json"), JSON.stringify(response));
}

main().catch((err) => {
	writeFileSync(join(stateDir, "error.log"), String(err?.stack ?? err));
	process.exitCode = 1;
});
