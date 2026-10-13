"use client";

import { FormEvent, useState } from "react";
import { X } from "lucide-react";
import { ApiError, createBeach, type Beach } from "@/lib/api";

interface BeachSuggestionDialogProps {
  accessToken: string;
  onClose: () => void;
  onCreated: (beach: Beach) => void;
}

export default function BeachSuggestionDialog({
  accessToken,
  onClose,
  onCreated,
}: BeachSuggestionDialogProps) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setLoading(true);
    setError("");
    const formData = new FormData(event.currentTarget);

    try {
      const beach = await createBeach(
        {
          name: String(formData.get("name") ?? "").trim(),
          latitude: Number(formData.get("latitude")),
          longitude: Number(formData.get("longitude")),
          description: String(formData.get("description") ?? "").trim(),
        },
        accessToken,
      );
      onCreated(beach);
    } catch (caught) {
      setError(
        caught instanceof ApiError
          ? caught.message
          : "Could not reach the Shaka API. Please try again.",
      );
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="dialog-backdrop" onMouseDown={onClose}>
      <section
        className="dialog-card suggestion-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="suggestion-dialog-title"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <button className="dialog-close" aria-label="Close" onClick={onClose}>
          <X size={18} />
        </button>
        <div className="dialog-eyebrow">LOCAL KNOWLEDGE MATTERS</div>
        <h2 id="suggestion-dialog-title">Add a beach.</h2>
        <p className="dialog-copy">Share a spot with the Shaka community.</p>
        <form className="dialog-form" onSubmit={submit}>
          <label>
            Beach name
            <input name="name" maxLength={100} required />
          </label>
          <div className="coordinate-fields">
            <label>
              Latitude
              <input
                name="latitude"
                type="number"
                min={-90}
                max={90}
                step="any"
                required
              />
            </label>
            <label>
              Longitude
              <input
                name="longitude"
                type="number"
                min={-180}
                max={180}
                step="any"
                required
              />
            </label>
          </div>
          <label>
            Description
            <textarea name="description" rows={3} maxLength={500} />
          </label>
          {error && <p className="dialog-error" role="alert">{error}</p>}
          <button className="dialog-submit" type="submit" disabled={loading}>
            {loading ? "Sharing…" : "Share this beach"}
          </button>
        </form>
      </section>
    </div>
  );
}
