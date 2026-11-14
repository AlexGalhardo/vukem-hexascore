import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, test, vi } from "vitest";

import Index, { type TrendingData } from "./Index";

vi.mock("../../lib/useT", () => ({
	useT: () => (key: string, p?: Record<string, unknown>) =>
		p ? `${key}:${Object.values(p).join(",")}` : key,
}));
vi.mock("../../components/Seo", () => ({ default: () => null }));
vi.mock("@inertiajs/react", () => ({
	Link: ({ href, children }: { href: string; children: ReactNode }) => (
		<a href={href}>{children}</a>
	),
	usePage: () => ({ props: { locale: "en", coversBase: "/covers" } }),
}));

afterEach(cleanup);

const item = { id: "i1", type: "game" as const, slug: "zelda", title: "Zelda", cover: null };

test("an empty week shows the empty message", () => {
	const data: TrendingData = { week: "", computedAt: null, items: [], reviews: [] };
	render(<Index trending={data} />);

	expect(screen.getByText("social.trending.empty")).toBeTruthy();
	expect(screen.queryByRole("list")).toBeNull();
});

test("items and reviews are ranked lists with their activity", () => {
	const data: TrendingData = {
		week: "2026-11-15",
		computedAt: "2026-11-15T06:00:00Z",
		items: [{ rank: 1, score: 4.7, reviews: 1, likes: 2, comments: 3, item }],
		reviews: [
			{
				rank: 1,
				score: 4.7,
				likes: 2,
				comments: 3,
				review: {
					id: "r1",
					author: { nickname: "Critic", initial: "C" },
					item,
					stars: 8,
					body: [{ t: "text", text: "Worth it" }],
					spoiler: false,
					helpful: 0,
					createdAt: "2026-11-14T00:00:00Z",
					updatedAt: "2026-11-14T00:00:00Z",
				},
			},
		],
	};
	render(<Index trending={data} />);

	expect(screen.getAllByRole("list")).toHaveLength(2);
	expect(screen.getByText("social.trending.activity:1,2,3")).toBeTruthy();
	expect(screen.getByText("social.trending.reviewActivity:2,3")).toBeTruthy();
	expect(screen.getByText("Critic")).toBeTruthy();
});
