"use client";

import { FormEvent, useEffect, useState } from "react";
import Link from "next/link";
import {
  ArrowLeft,
  ArrowUpRight,
  LoaderCircle,
  MessageCircle,
  Plus,
  Send,
  Waves,
  X,
} from "lucide-react";
import AuthDialog from "@/components/auth-dialog";
import SiteHeader from "@/components/site-header";
import {
  ApiError,
  createForumPost,
  createForumThread,
  getForumCategories,
  getForumThread,
  getForumThreads,
  type ForumCategory,
  type ForumThread,
  type ForumThreadDetail,
} from "@/lib/api";
import { useAuthSession } from "@/lib/use-auth-session";

function timeAgo(value: string) {
  const elapsed = Math.max(0, Date.now() - new Date(value).getTime());
  const minutes = Math.floor(elapsed / 60_000);
  if (minutes < 2) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function initials(email: string) {
  return email.split("@")[0].slice(0, 1).toUpperCase() || "S";
}

export default function ForumPage() {
  const { session, notice, signIn, signOut, setNotice } = useAuthSession();
  const [categories, setCategories] = useState<ForumCategory[]>([]);
  const [threads, setThreads] = useState<ForumThread[]>([]);
  const [activeCategory, setActiveCategory] = useState("");
  const [selectedID, setSelectedID] = useState("");
  const [detail, setDetail] = useState<ForumThreadDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [error, setError] = useState("");
  const [authOpen, setAuthOpen] = useState(false);
  const [composeOpen, setComposeOpen] = useState(false);
  const [reply, setReply] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [announcement, setAnnouncement] = useState("");

  useEffect(() => {
    let active = true;
    getForumCategories()
      .then((result) => {
        if (active) setCategories(result);
      })
      .catch((caught: unknown) => {
        if (active) {
          setError(caught instanceof ApiError ? caught.message : "Could not load the community.");
        }
      });
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    let active = true;
    getForumThreads(activeCategory)
      .then((result) => {
        if (!active) return;
        setThreads(result);
        setDetail(null);
        setDetailLoading(Boolean(result[0]));
        setSelectedID(result[0]?.id ?? "");
      })
      .catch((caught: unknown) => {
        if (active) {
          setError(caught instanceof ApiError ? caught.message : "Could not load forum discussions.");
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [activeCategory]);

  useEffect(() => {
    if (!selectedID) return;
    let active = true;
    getForumThread(selectedID)
      .then((result) => {
        if (active) setDetail(result);
      })
      .catch((caught: unknown) => {
        if (active) {
          setError(caught instanceof ApiError ? caught.message : "Could not load this discussion.");
        }
      })
      .finally(() => {
        if (active) setDetailLoading(false);
      });
    return () => {
      active = false;
    };
  }, [selectedID]);

  const submitThread = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!session) {
      setComposeOpen(false);
      setAuthOpen(true);
      return;
    }
    const form = new FormData(event.currentTarget);
    setSubmitting(true);
    setError("");
    try {
      const thread = await createForumThread(
        {
          category_slug: String(form.get("category_slug") ?? ""),
          title: String(form.get("title") ?? "").trim(),
          body: String(form.get("body") ?? "").trim(),
        },
        session.accessToken,
      );
      setComposeOpen(false);
      setAnnouncement("Your discussion is live.");
      setActiveCategory(thread.category_slug);
      setSelectedID(thread.id);
      setDetail({ thread, posts: [] });
      const updated = await getForumThreads(thread.category_slug);
      setThreads(updated);
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "Could not publish your discussion.");
    } finally {
      setSubmitting(false);
    }
  };

  const submitReply = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!session || !detail) {
      setAuthOpen(true);
      return;
    }
    const body = reply.trim();
    if (!body) return;
    setSubmitting(true);
    try {
      await createForumPost(detail.thread.id, body, session.accessToken);
      setReply("");
      const updated = await getForumThread(detail.thread.id);
      setDetail(updated);
      const updatedThreads = await getForumThreads(activeCategory);
      setThreads(updatedThreads);
      setAnnouncement("Your reply has been added.");
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "Could not add your reply.");
    } finally {
      setSubmitting(false);
    }
  };

  const activeCategoryName =
    categories.find((category) => category.slug === activeCategory)?.name ?? "All discussions";
  const selectCategory = (slug: string) => {
    setLoading(true);
    setDetail(null);
    setSelectedID("");
    setActiveCategory(slug);
  };

  return (
    <main className="forum-page">
      <SiteHeader
        current="forum"
        session={session}
        onSignIn={() => setAuthOpen(true)}
        onSignOut={signOut}
      />
      <section className="forum-intro">
        <div>
          <span className="shop-eyebrow"><i /> THE SHAKA LINEUP</span>
          <h1>Talk story.<br /><em>Find your people.</em></h1>
          <p>Local breaks, gear wisdom and the small things that make a good session.</p>
        </div>
        <div className="forum-intro-mark"><Waves size={33} /><span>COMMUNITY<br />ON THE COAST</span></div>
      </section>

      {(notice || announcement || error) && (
        <div className={`commerce-notice ${error ? "error" : ""}`} role="status">
          <span>{error || announcement || notice}</span>
          <button
            aria-label="Dismiss message"
            onClick={() => { setError(""); setAnnouncement(""); setNotice(""); }}
          >
            <X size={15} />
          </button>
        </div>
      )}

      <section className="forum-layout">
        <aside className="forum-sidebar">
          <button
            className="new-thread-button"
            onClick={() => session ? setComposeOpen(true) : setAuthOpen(true)}
          >
            <Plus size={16} /> Start a discussion
          </button>
          <div className="forum-sidebar-label">YOUR COMMUNITY</div>
          <button
            className={`forum-category-button ${activeCategory === "" ? "active" : ""}`}
            onClick={() => selectCategory("")}
          >
            <span>All discussions</span><b>{threads.length}</b>
          </button>
          {categories.map((category) => (
            <button
              key={category.slug}
              className={`forum-category-button ${activeCategory === category.slug ? "active" : ""}`}
              onClick={() => selectCategory(category.slug)}
            >
              <span>{category.name}</span><b>{category.thread_count}</b>
            </button>
          ))}
          <div className="forum-guidelines">
            <span>✳</span>
            <strong>Keep it welcoming.</strong>
            <p>Share what you know, respect local spots and leave the beach better than you found it.</p>
          </div>
        </aside>

        <section className="thread-list-panel">
          <div className="forum-panel-heading">
            <div><span className="shop-eyebrow">COMMUNITY BOARD · SAMPLE DISCUSSIONS</span><h2>{activeCategoryName}</h2></div>
            <span>{threads.length} topics</span>
          </div>
          {loading ? (
            <div className="forum-empty"><LoaderCircle size={19} className="spin" /> Loading discussions…</div>
          ) : threads.length === 0 ? (
            <div className="forum-empty">
              <MessageCircle size={24} />
              <strong>First one in?</strong>
              <span>Start a discussion and get the conversation rolling.</span>
            </div>
          ) : (
            <div className="thread-list">
              {threads.map((thread) => (
                <button
                  key={thread.id}
                  className={`thread-row ${thread.id === selectedID ? "active" : ""}`}
                  onClick={() => {
                    if (thread.id === selectedID) return;
                    setDetail(null);
                    setDetailLoading(true);
                    setSelectedID(thread.id);
                  }}
                >
                  <span className="thread-avatar">{initials(thread.author_email)}</span>
                  <span className="thread-row-copy">
                    <span className="thread-category-tag">
                      {categories.find((item) => item.slug === thread.category_slug)?.name ?? thread.category_slug}
                    </span>
                    <strong>{thread.title}</strong>
                    <span>{thread.author_email} · {timeAgo(thread.updated_at)}</span>
                  </span>
                  <span className="thread-replies"><MessageCircle size={14} /> {thread.reply_count}</span>
                  <ArrowUpRight className="thread-open-icon" size={15} />
                </button>
              ))}
            </div>
          )}
        </section>

        <section className="thread-detail-panel">
          {detailLoading ? (
            <div className="forum-empty"><LoaderCircle size={19} className="spin" /> Opening topic…</div>
          ) : detail ? (
            <>
              <div className="thread-detail-scroll">
                <button className="thread-back-button" onClick={() => setSelectedID("")}>
                  <ArrowLeft size={14} /> All topics
                </button>
                <span className="thread-category-tag">
                  {categories.find((item) => item.slug === detail.thread.category_slug)?.name ?? detail.thread.category_slug}
                </span>
                <h2>{detail.thread.title}</h2>
                <div className="thread-author-line">
                  <span className="thread-avatar">{initials(detail.thread.author_email)}</span>
                  <span><b>{detail.thread.author_email}</b><small>Started {timeAgo(detail.thread.created_at)}</small></span>
                </div>
                <p className="thread-body">{detail.thread.body}</p>
                <div className="thread-replies-heading">
                  <span>THE CONVERSATION</span><b>{detail.posts.length} replies</b>
                </div>
                {detail.posts.map((post) => (
                  <article className="forum-post" key={post.id}>
                    <span className="thread-avatar">{initials(post.author_email)}</span>
                    <div>
                      <div className="post-author"><b>{post.author_email}</b><time>{timeAgo(post.created_at)}</time></div>
                      <p>{post.body}</p>
                    </div>
                  </article>
                ))}
                {detail.posts.length === 0 && <p className="no-replies">No replies yet. Add the first thought.</p>}
              </div>
              <form className="reply-form" onSubmit={(event) => void submitReply(event)}>
                <textarea
                  value={reply}
                  onChange={(event) => setReply(event.target.value)}
                  placeholder={session ? "Add something helpful…" : "Sign in to join the conversation"}
                  maxLength={10000}
                  rows={2}
                  disabled={!session}
                />
                {!session && <button type="button" onClick={() => setAuthOpen(true)}>Sign in to reply</button>}
                {session && (
                  <button type="submit" disabled={!reply.trim() || submitting}>
                    {submitting ? <LoaderCircle size={15} className="spin" /> : <Send size={15} />}
                    Reply
                  </button>
                )}
              </form>
            </>
          ) : (
            <div className="forum-empty">
              <MessageCircle size={26} />
              <strong>Select a conversation</strong>
              <span>Choose a topic from the board to read and reply.</span>
            </div>
          )}
        </section>
      </section>

      <footer className="commerce-footer forum-footer">
        <span>shaka<span>.</span> SURF CLUB</span>
        <span>GOOD TIDES, GOOD PEOPLE.</span>
        <Link href="/conditions">Check today’s conditions <ArrowUpRight size={13} /></Link>
      </footer>

      {authOpen && (
        <AuthDialog
          onClose={() => setAuthOpen(false)}
          onAuthenticated={(nextSession) => { signIn(nextSession); setAuthOpen(false); }}
        />
      )}
      {composeOpen && (
        <div className="dialog-backdrop" onMouseDown={() => setComposeOpen(false)}>
          <section
            className="dialog-card compose-dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby="compose-title"
            onMouseDown={(event) => event.stopPropagation()}
          >
            <button className="dialog-close" aria-label="Close" onClick={() => setComposeOpen(false)}><X size={18} /></button>
            <div className="dialog-eyebrow">START THE CONVERSATION</div>
            <h2 id="compose-title">What’s on your mind?</h2>
            <p className="dialog-copy">Ask a question, share a local tip or start planning a session.</p>
            <form className="dialog-form" onSubmit={(event) => void submitThread(event)}>
              <label>
                Category
                <select name="category_slug" required defaultValue={activeCategory || categories[0]?.slug}>
                  {categories.map((item) => <option key={item.slug} value={item.slug}>{item.name}</option>)}
                </select>
              </label>
              <label>Topic title<input name="title" maxLength={180} required /></label>
              <label>Your post<textarea name="body" rows={5} maxLength={10000} required /></label>
              <button className="dialog-submit" type="submit" disabled={submitting || categories.length === 0}>
                {submitting ? "Publishing…" : "Publish discussion"}
              </button>
            </form>
          </section>
        </div>
      )}
    </main>
  );
}
