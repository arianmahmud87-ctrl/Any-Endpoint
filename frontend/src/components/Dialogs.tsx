import type { ReactNode } from "react";
import { AlertTriangle } from "lucide-react";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Btn, CopyBtn } from "./kit";

export function Confirm({ open, onOpenChange, title, body, confirmLabel, onConfirm, busy }: {
  open: boolean; onOpenChange: (o: boolean) => void; title: string; body: string; confirmLabel: string; onConfirm: () => void; busy?: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm rounded-2xl">
        <DialogHeader>
          <DialogTitle className="text-base">{title}</DialogTitle>
          <DialogDescription className="text-[13px]">{body}</DialogDescription>
        </DialogHeader>
        <div className="mt-2 flex justify-end gap-2">
          <Btn variant="ghost" onClick={() => onOpenChange(false)}>Cancel</Btn>
          <Btn variant="danger" disabled={busy} onClick={onConfirm}>{busy ? "Working…" : confirmLabel}</Btn>
        </div>
      </DialogContent>
    </Dialog>
  );
}

/** Shows a secret exactly once; value lives only in parent transient state. */
export function RevealOnce({ open, onClose, title, value, note, extra }: {
  open: boolean; onClose: () => void; title: string; value: string; note: string; extra?: ReactNode;
}) {
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-md rounded-2xl" onInteractOutside={(e) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle className="text-base">{title}</DialogTitle>
          <DialogDescription className="text-[13px]">Copy it now. You won't be able to see it again.</DialogDescription>
        </DialogHeader>
        <div className="mono break-all rounded-lg border bg-[hsl(var(--canvas))] p-3 text-[12.5px]">{value}</div>
        {extra}
        <div className="flex items-start gap-2 rounded-lg bg-[hsl(var(--warn)/.1)] p-3 text-[12.5px]">
          <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[hsl(var(--warn))]" />
          <span>{note}</span>
        </div>
        <div className="flex justify-end gap-2">
          <CopyBtn value={value} />
          <Btn variant="primary" onClick={onClose}>Done</Btn>
        </div>
      </DialogContent>
    </Dialog>
  );
}
