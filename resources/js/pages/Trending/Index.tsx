import { usePage } from "@inertiajs/react";
import type { Cover } from "../../components/CoverThumb";
import { ReviewCard, type ReviewEntry } from "../../components/item/Reviews";
import type { ItemRef } from "../../components/library/ItemLink";
import ShelfCard from "../../components/library/ShelfCard";
import Seo from "../../components/Seo";
import { useT } from "../../lib/useT";
import type { SharedProps } from "../../types";

type TrendingItemRef = Omit<ItemRef, "year" | "cover"> & { cover: Cover | null };

/** app/social/trending.go: TrendingItem. */
export interface TrendingItem {
	rank: number;
	score: number;
	reviews: number;
	likes: number;
	comments: number;
	item: TrendingItemRef;
}

/** app/social/trending.go: TrendingReview. */
export interface TrendingReview {
	rank: number;
	score: number;
	likes: number;
	comments: number;
	review: ReviewEntry & { item: TrendingItemRef };
}

export interface TrendingData {
	week: string;
	computedAt: string | null;
	items: TrendingItem[];
	reviews: TrendingReview[];
}

function Rank({ n }: { n: number }) {
	return (
		<span
			aria-hidden="true"
			className="w-8 shrink-0 pt-3 font-display text-2xl font-bold tabular-nums text-muted"
		>
			{n}
		</span>
	);
}

export default function Index({ trending }: { trending: TrendingData }) {
	const t = useT();
	const { locale } = usePage<SharedProps>().props;
	const empty = trending.items.length === 0 && trending.reviews.length === 0;
	const week = trending.week
		? new Date(`${trending.week}T12:00:00`).toLocaleDateString(locale)
		: "";

	return (
		<>
			<Seo title={t("social.trending.title")} description={t("social.trending.intro")} />
			<h1 className="mt-6 font-display text-5xl leading-none font-bold tracking-[-0.02em] md:mt-10 md:text-[64px]">
				{t("social.trending.title")}
			</h1>
			<p className="mt-4 max-w-3xl text-muted">
				{t("social.trending.intro")}
				{week && ` ${t("social.trending.weekOf", { date: week })}.`}
			</p>
			{empty ? (
				<p className="mt-8 max-w-3xl border-t border-border pt-6 font-display text-2xl">
					{t("social.trending.empty")}
				</p>
			) : (
				<div className="mt-8 grid gap-10 lg:grid-cols-2">
					<section aria-labelledby="trending-items">
						<h2 id="trending-items" className="font-display text-3xl font-bold">
							{t("social.trending.items")}
						</h2>
						<ol className="mt-4 divide-y divide-border border-t border-border">
							{trending.items.map((e) => (
								<li key={e.item.id} className="flex gap-3 py-4">
									<Rank n={e.rank} />
									<div className="min-w-0 flex-1">
										<ShelfCard item={{ ...e.item, year: null }}>
											<p className="mt-2 text-sm text-muted">
												{t("social.trending.activity", {
													reviews: e.reviews,
													likes: e.likes,
													comments: e.comments,
												})}
											</p>
										</ShelfCard>
									</div>
								</li>
							))}
						</ol>
					</section>
					<section aria-labelledby="trending-reviews">
						<h2 id="trending-reviews" className="font-display text-3xl font-bold">
							{t("social.trending.reviews")}
						</h2>
						<ol className="mt-4 divide-y divide-border border-t border-border">
							{trending.reviews.map((e) => (
								<li key={e.review.id} className="flex gap-3 py-4">
									<Rank n={e.rank} />
									<div className="min-w-0 flex-1">
										<ShelfCard item={{ ...e.review.item, year: null }}>
											<ReviewCard review={e.review} />
											<p className="text-sm text-muted">
												{t("social.trending.reviewActivity", {
													likes: e.likes,
													comments: e.comments,
												})}
											</p>
										</ShelfCard>
									</div>
								</li>
							))}
						</ol>
					</section>
				</div>
			)}
		</>
	);
}
