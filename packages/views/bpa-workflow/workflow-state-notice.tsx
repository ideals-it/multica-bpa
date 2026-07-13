import { Button } from "@multica/ui/components/ui/button";
import type { IssueMetadata } from "@multica/core/types";

type ApprovalDecision = "approved" | "rejected";

export function WorkflowStateNotice({
  metadata,
  onDecision,
  pending = false,
}: {
  metadata: IssueMetadata;
  onDecision?: (decision: ApprovalDecision) => void;
  pending?: boolean;
}) {
  if (metadata["bpa.template"] !== "production") return null;

  const waitingFor = metadata["bpa.waiting_for"];
  const summary = typeof metadata["bpa.approval_summary"] === "string"
    ? metadata["bpa.approval_summary"]
    : "";

  if (waitingFor === "human_approval") {
    return (
      <section className="mt-5 rounded-lg border border-amber-300 bg-amber-50 p-4 text-sm dark:border-amber-900 dark:bg-amber-950/30">
        <h2 className="font-semibold text-foreground">Потрібне погодження</h2>
        <p className="mt-1 text-muted-foreground">{summary || "Team Lead просить підтвердити наступну дію."}</p>
        {onDecision && (
          <div className="mt-3 flex gap-2">
            <Button size="sm" onClick={() => onDecision("approved")} disabled={pending}>
              Погодити
            </Button>
            <Button size="sm" variant="outline" onClick={() => onDecision("rejected")} disabled={pending}>
              Відхилити
            </Button>
          </div>
        )}
      </section>
    );
  }

  if (waitingFor === "quality") {
    return <section className="mt-5 rounded-lg border p-4 text-sm"><strong>Перевірка результату</strong><p className="mt-1 text-muted-foreground">Quality перевіряє виконану роботу.</p></section>;
  }

  if (waitingFor === "lead") {
    return <section className="mt-5 rounded-lg border p-4 text-sm"><strong>Наступний етап</strong><p className="mt-1 text-muted-foreground">Team Lead готує наступний крок.</p></section>;
  }

  return null;
}
