import { describe, expect, it } from "vitest";
import {
	api,
	assertSuccess,
	createDomain,
	registerUser,
	unique,
} from "../helpers";

interface LayoutGroup {
	uuid: string;
	name: string;
	color: string;
	sort_order: number;
}

interface LayoutItem {
	domain_uuid: string;
	group_uuid: string | null;
	sort_order: number;
}

interface Layout {
	groups: LayoutGroup[];
	items: LayoutItem[];
}

async function getLayout(token: string): Promise<Layout> {
	const result = await api<Layout>("/api/v1/domain/layout/get", {
		token,
		body: {},
	});
	return assertSuccess(result);
}

async function saveLayout(token: string, body: Layout): Promise<Layout> {
	const result = await api<Layout>("/api/v1/domain/layout/save", {
		token,
		body,
	});
	return assertSuccess(result);
}

describe("domain personal layout module", () => {
	it("persists folders per user and fills joined domains on read", async () => {
		const owner = await registerUser("layout");
		const other = await registerUser("layout");
		const first = await createDomain(
			owner.access_token,
			unique("layout_first"),
		);
		const second = await createDomain(
			owner.access_token,
			unique("layout_second"),
		);

		const initial = await getLayout(owner.access_token);
		expect(initial.groups).toEqual([]);
		expect(initial.items.map((item) => item.domain_uuid).sort()).toEqual(
			[first.uuid, second.uuid].sort(),
		);

		const saved = await saveLayout(owner.access_token, {
			groups: [
				{ uuid: "folder-1", name: "工作", color: "lime", sort_order: 0 },
			],
			items: [
				{ domain_uuid: first.uuid, group_uuid: "folder-1", sort_order: 0 },
				{ domain_uuid: second.uuid, group_uuid: null, sort_order: 0 },
			],
		});
		expect(saved.groups[0]?.name).toBe("工作");
		expect(
			saved.items.find((i) => i.domain_uuid === first.uuid)?.group_uuid,
		).toBe("folder-1");

		const reloaded = await getLayout(owner.access_token);
		expect(reloaded.groups).toEqual(saved.groups);
		expect(reloaded.items).toEqual(saved.items);

		const otherLayout = await getLayout(other.access_token);
		expect(otherLayout.groups).toEqual([]);
		expect(otherLayout.items).toEqual([]);
	});

	it("rejects a domain the user has not joined", async () => {
		const owner = await registerUser("layout");
		const stranger = await registerUser("layout");
		const foreign = await createDomain(
			stranger.access_token,
			unique("layout_foreign"),
		);
		const result = await api("/api/v1/domain/layout/save", {
			token: owner.access_token,
			body: {
				groups: [],
				items: [{ domain_uuid: foreign.uuid, group_uuid: null, sort_order: 0 }],
			},
		});
		expect(result.code).not.toBe(0);
	});

	it("rejects invalid group names and colors", async () => {
		const owner = await registerUser("layout");
		const domain = await createDomain(
			owner.access_token,
			unique("layout_invalid"),
		);
		for (const group of [
			{ uuid: "g1", name: "   ", color: "lime", sort_order: 0 },
			{ uuid: "g1", name: "工作", color: "not-a-token", sort_order: 0 },
		]) {
			const result = await api("/api/v1/domain/layout/save", {
				token: owner.access_token,
				body: {
					groups: [group],
					items: [
						{ domain_uuid: domain.uuid, group_uuid: "g1", sort_order: 0 },
					],
				},
			});
			expect(result.code, JSON.stringify(result)).not.toBe(0);
		}
	});
});
