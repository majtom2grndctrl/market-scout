import { listOpenPostings } from "@/lib/db/postings";
import { workplaceTypeLabel } from "@/lib/format";

export default async function PostingsPage() {
  const postings = await listOpenPostings();

  return (
    <main>
      <h1>Open postings</h1>
      <ul>
        {postings.map((posting) => {
          const workplaceLabel = workplaceTypeLabel(
            posting.workplace_type_resolved,
            posting.workplace_type_source,
          );

          return (
            <li key={posting.job_posting_id}>
              <p>Company: {posting.company_name}</p>
              <p>Title: {posting.title ?? ""}</p>
              <p>Location: {posting.location_text ?? ""}</p>
              {workplaceLabel !== null && <p>Workplace: {workplaceLabel}</p>}
              <p>Seniority: {posting.seniority ?? ""}</p>
              <p>Open: {posting.run_started_at.toISOString()}</p>
            </li>
          );
        })}
      </ul>
    </main>
  );
}
