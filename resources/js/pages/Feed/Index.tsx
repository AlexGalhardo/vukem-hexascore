import { Link, usePage } from "@inertiajs/react";
import Seo from "../../components/Seo";
import ActivityCard, { type ActivityEntry } from "../../components/social/ActivityCard";
import { btnClass } from "../../components/ui/Btn";
import { localized } from "../../lib/route";
import { useT } from "../../lib/useT";
import type { SharedProps } from "../../types";

interface Props {
	events: ActivityEntry[];
	nextCursor: string;
}

export default function Index({ events, nextCursor }: Props) {
	const t = useT();
	const { locale } = usePage<SharedProps>().props;

	return (
		<>
			<Seo title={t("social.feed.title")} noindex />
			<h1 className="mt-6 font-display text-5xl leading-none font-bold tracking-[-0.02em] md:mt-10 md:text-[64px]">
				{t("social.feed.title")}
			</h1>
			<Link
				href={localized("/trending", locale)}
				className="mt-4 inline-block font-medium underline"
			>
				{t("social.trending.link")}
			</Link>
			{events.length === 0 ? (
				<p className="mt-8 max-w-3xl border-t border-border pt-6 font-display text-2xl">
					{t("social.feed.empty")}
				</p>
			) : (
				<ul className="mt-8 max-w-3xl divide-y divide-border border-t border-border">
					{events.map((e) => (
						<li key={e.id}>
							<ActivityCard entry={e} />
						</li>
					))}
				</ul>
			)}
			{nextCursor && (
				<Link
					href={`?cursor=${nextCursor}`}
					className={btnClass("secondary", "mt-6")}
					preserveScroll
				>
					{t("social.feed.more")}
				</Link>
			)}
		</>
	);
}
