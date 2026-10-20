"use client";

import dynamic from "next/dynamic";
import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ArrowUpRight,
  Bell,
  Compass,
  LogOut,
  MapPin,
  Navigation,
  Plus,
  Search,
  Waves,
  Wind,
  X,
} from "lucide-react";
import AuthDialog from "@/components/auth-dialog";
import BeachSuggestionDialog from "@/components/beach-suggestion-dialog";
import {
  ApiError,
  getBeaches,
  type AuthSession,
  type Beach,
} from "@/lib/api";
import { useAuthSession } from "@/lib/use-auth-session";

const SurfMap = dynamic(() => import("@/components/surf-map"), {
  ssr: false,
  loading: () => (
    <div className="map-loading">
      <span className="map-loading-pulse" />
      <span>Charting the coastline…</span>
    </div>
  ),
});

interface SwellAlert {
  beach_id: string;
  beach_name: string;
  wave_height: number;
  timestamp: string;
}

type NotificationStatus = "connecting" | "connected" | "reconnecting";

interface UserCoordinates {
  latitude: number;
  longitude: number;
}

const notificationBaseUrl =
  process.env.NEXT_PUBLIC_NOTIFICATION_BASE_URL ??
  (process.env.NODE_ENV === "production"
    ? "https://151.145.82.171.nip.io"
    : "http://localhost:8082");

function distanceInKm(
  start: UserCoordinates,
  end: Pick<Beach, "latitude" | "longitude">,
) {
  const radians = (degrees: number) => (degrees * Math.PI) / 180;
  const latitudeDelta = radians(end.latitude - start.latitude);
  const longitudeDelta = radians(end.longitude - start.longitude);
  const haversine =
    Math.sin(latitudeDelta / 2) ** 2 +
    Math.cos(radians(start.latitude)) *
      Math.cos(radians(end.latitude)) *
      Math.sin(longitudeDelta / 2) ** 2;
  return 6371 * 2 * Math.atan2(Math.sqrt(haversine), Math.sqrt(1 - haversine));
}

function formatUpdatedAt(value: string) {
  const updated = new Date(value);
  if (Number.isNaN(updated.getTime())) return "Conditions updated";
  return `Updated ${updated.toLocaleTimeString([], {
    hour: "numeric",
    minute: "2-digit",
  })}`;
}

function getSurfStatus(height: number) {
  if (height >= 1.5) return { label: "Pumping", color: "coral" };
  if (height >= 0.8) return { label: "Looking good", color: "green" };
  return { label: "Small", color: "blue" };
}

function compassDirection(degrees: number) {
  const directions = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"];
  return directions[Math.round(degrees / 45) % directions.length];
}

export default function ConditionsPage() {
  const [beaches, setBeaches] = useState<Beach[]>([]);
  const [selectedBeach, setSelectedBeach] = useState<Beach | null>(null);
  const [mapFocus, setMapFocus] = useState<Beach | null>(null);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [locating, setLocating] = useState(false);
  const [userLocation, setUserLocation] = useState<UserCoordinates | null>(null);
  const [error, setError] = useState("");
  const [locationMessage, setLocationMessage] = useState("");
  const {
    session: authSession,
    ready: authReady,
    notice: authNotice,
    signIn,
    signOut: handleSignOut,
  } = useAuthSession();
  const [authDialogOpen, setAuthDialogOpen] = useState(false);
  const [suggestionDialogOpen, setSuggestionDialogOpen] = useState(false);
  const [notificationStatus, setNotificationStatus] =
    useState<NotificationStatus>("connecting");
  const [swellAlert, setSwellAlert] = useState<SwellAlert | null>(null);

  const applyBeachResults = useCallback((results: Beach[]) => {
    setBeaches(results);
    setSelectedBeach((current) =>
      results.find((beach) => beach.id === current?.id) ?? results[0] ?? null,
    );
  }, []);

  const getLoadErrorMessage = (caught: unknown) =>
    caught instanceof ApiError
      ? caught.message
      : "Could not connect to the Shaka API. Check that the gateway is running.";

  const loadBeaches = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      applyBeachResults(await getBeaches());
    } catch (caught) {
      setError(getLoadErrorMessage(caught));
    } finally {
      setLoading(false);
    }
  }, [applyBeachResults]);

  useEffect(() => {
    let active = true;
    getBeaches()
      .then((results) => {
        if (active) applyBeachResults(results);
      })
      .catch((caught: unknown) => {
        if (active) setError(getLoadErrorMessage(caught));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [applyBeachResults]);

  useEffect(() => {
    const events = new EventSource(`${notificationBaseUrl.replace(/\/+$/, "")}/events`);
    events.onopen = () => setNotificationStatus("connected");
    events.onerror = () => setNotificationStatus("reconnecting");
    const receiveAlert = (event: Event) => {
      try {
        const parsed: unknown = JSON.parse((event as MessageEvent<string>).data);
        if (
          typeof parsed !== "object" ||
          parsed === null ||
          !("beach_id" in parsed) ||
          typeof parsed.beach_id !== "string" ||
          !("beach_name" in parsed) ||
          typeof parsed.beach_name !== "string" ||
          !("wave_height" in parsed) ||
          typeof parsed.wave_height !== "number" ||
          !Number.isFinite(parsed.wave_height) ||
          !("timestamp" in parsed) ||
          typeof parsed.timestamp !== "string" ||
          Number.isNaN(Date.parse(parsed.timestamp))
        ) {
          throw new Error("notification payload has an invalid shape");
        }
        setSwellAlert({
          beach_id: parsed.beach_id,
          beach_name: parsed.beach_name,
          wave_height: parsed.wave_height,
          timestamp: parsed.timestamp,
        });
      } catch (caught) {
        console.error("Could not process a swell alert event:", caught);
      }
    };
    events.addEventListener("swell-alert", receiveAlert);
    return () => {
      events.removeEventListener("swell-alert", receiveAlert);
      events.close();
    };
  }, []);

  useEffect(() => {
    if (!swellAlert) return;
    const timeout = window.setTimeout(() => setSwellAlert(null), 8_000);
    return () => window.clearTimeout(timeout);
  }, [swellAlert]);

  const handleAuthenticated = (session: AuthSession) => {
    signIn(session);
    setAuthDialogOpen(false);
  };

  const handleBeachCreated = (beach: Beach) => {
    setBeaches((current) => [beach, ...current.filter((item) => item.id !== beach.id)]);
    setSelectedBeach(beach);
    setMapFocus(beach);
    setSuggestionDialogOpen(false);
  };

  const sortedBeaches = useMemo(() => {
    if (!userLocation) return beaches;
    return [...beaches].sort(
      (left, right) =>
        distanceInKm(userLocation, left) - distanceInKm(userLocation, right),
    );
  }, [beaches, userLocation]);

  const visibleBeaches = sortedBeaches.filter((beach) =>
    beach.name.toLowerCase().includes(search.trim().toLowerCase()),
  );

  const findClosestBeach = () => {
    setLocationMessage("");
    if (!navigator.geolocation) {
      setLocationMessage("Location is not supported by this browser.");
      return;
    }

    setLocating(true);
    navigator.geolocation.getCurrentPosition(
      ({ coords }) => {
        const location = {
          latitude: coords.latitude,
          longitude: coords.longitude,
        };
        setUserLocation(location);
        const closest = [...beaches].sort(
          (left, right) =>
            distanceInKm(location, left) - distanceInKm(location, right),
        )[0];
        if (closest) {
          setSelectedBeach(closest);
          setMapFocus(closest);
          setLocationMessage(`Sorted by distance · closest: ${closest.name}`);
        } else {
          setLocationMessage("Your location was found, but no beaches are loaded.");
        }
        setLocating(false);
      },
      (geolocationError) => {
        setLocating(false);
        setLocationMessage(
          geolocationError.code === geolocationError.PERMISSION_DENIED
            ? "Allow location access to find your closest beach."
            : "Could not determine your location. Try again.",
        );
      },
      { enableHighAccuracy: true, timeout: 12_000, maximumAge: 60_000 },
    );
  };

  return (
    <main className="dashboard-shell">
      <aside className="sidebar">
        <div className="brand-row">
          <div className="brand-mark">
            <Waves size={21} strokeWidth={2.2} />
          </div>
          <div>
            <div className="brand-name">shaka<span>.</span></div>
            <div className="brand-caption">SURF CLUB</div>
          </div>
          <span className="live-pill"><i /> LIVE</span>
        </div>
        <nav className="dashboard-quick-links" aria-label="Shaka features">
          <Link href="/">Shop gear</Link>
          <Link href="/forum">Community forum</Link>
        </nav>

        <div className="sidebar-heading">
          <div>
            <div className="eyebrow">ISRAEL · MEDITERRANEAN</div>
            <h1>Find your<br /><span>next wave.</span></h1>
          </div>
          <div className="sun-icon" aria-hidden="true">☀</div>
        </div>

        <button
          className="location-button"
          onClick={findClosestBeach}
          disabled={locating}
        >
          <span className="location-icon">
            <Navigation size={17} fill="currentColor" />
          </span>
          <span>{locating ? "Finding your spot…" : "Find closest beach"}</span>
          <ArrowUpRight className="location-arrow" size={16} />
        </button>
        {locationMessage && <p className="location-message">{locationMessage}</p>}

        <div className="section-title">
          <span>BEACHES</span>
          <span className="beach-count">{beaches.length.toString().padStart(2, "0")}</span>
        </div>
        <label className="search-box">
          <Search size={16} />
          <input
            aria-label="Search beaches"
            placeholder="Find a beach…"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          <kbd>/</kbd>
        </label>

        <div className="beach-list">
          {loading && <div className="list-message">Checking the coast…</div>}
          {!loading && error && (
            <div className="list-error">
              <p>{error}</p>
              <button onClick={() => void loadBeaches()}>Try again</button>
            </div>
          )}
          {!loading && !error && visibleBeaches.length === 0 && (
            <div className="list-message">No beaches match that search.</div>
          )}
          {!loading &&
            !error &&
            visibleBeaches.map((beach, index) => {
              const status = getSurfStatus(beach.wave_height);
              const selected = selectedBeach?.id === beach.id;
              return (
                <button
                  className={`beach-card ${selected ? "selected" : ""}`}
                  key={beach.id}
                  onClick={() => {
                    setSelectedBeach(beach);
                    setMapFocus(beach);
                  }}
                >
                  <span className={`beach-number ${selected ? "active" : ""}`}>
                    {String(index + 1).padStart(2, "0")}
                  </span>
                  <span className="beach-info">
                    <span className="beach-name">{beach.name}</span>
                    <span className="beach-description">{beach.description || "Coastal break"}</span>
                    <span className={`condition-status ${status.color}`}>
                      <i /> {status.label}
                    </span>
                  </span>
                  <span className="beach-wave">
                    <strong>{beach.wave_height.toFixed(1)}<small>m</small></strong>
                    <span>WAVE</span>
                    {userLocation && (
                      <span className="beach-distance">
                        {distanceInKm(userLocation, beach).toFixed(1)} km
                      </span>
                    )}
                  </span>
                </button>
              );
            })}
        </div>

        <div className="sidebar-footer">
          <span className="weather-mini"><Wind size={14} /> ON-SHORE CHECK</span>
          <span>MADE FOR THE SEA <span className="footer-spark">✳</span></span>
        </div>
      </aside>

      <section className="map-column">
        <header className="map-topbar">
          <div className="map-breadcrumb">
            <span className="breadcrumb-dot" />
            <span>THE MEDITERRANEAN</span>
            <span className="breadcrumb-slash">/</span>
            <strong>ISRAEL COAST</strong>
          </div>
          <div className="topbar-right">
            <span className="conditions-time">
              {selectedBeach ? formatUpdatedAt(selectedBeach.updated_at) : "LIVE CONDITIONS"}
            </span>
            <span className={`notification-state ${notificationStatus}`}>
              <i />
              <span>{notificationStatus === "connected" ? "ALERTS LIVE" : "ALERTS RECONNECTING"}</span>
            </span>
            <button
              className="refresh-button"
              aria-label="Refresh beach conditions"
              onClick={() => void loadBeaches()}
              disabled={loading}
            >
              <Compass size={17} className={loading ? "spin" : ""} />
            </button>
            {authSession ? (
              <>
                <button
                  className="suggest-beach-button"
                  onClick={() => setSuggestionDialogOpen(true)}
                >
                  <Plus size={15} /> <span>Suggest a beach</span>
                </button>
                <button
                  className="account-button"
                  onClick={handleSignOut}
                  title={`Sign out ${authSession.email}`}
                  aria-label={`Sign out ${authSession.email}`}
                >
                  <span className="avatar">{authSession.email[0]?.toUpperCase() ?? "S"}</span>
                  <LogOut size={14} />
                </button>
              </>
            ) : (
              <button
                className="sign-in-button"
                onClick={() => setAuthDialogOpen(true)}
                disabled={!authReady}
              >
                Sign in
              </button>
            )}
          </div>
        </header>
        {authNotice && <p className="auth-notice" role="status">{authNotice}</p>}
        {swellAlert && (
          <aside className="swell-toast" role="alert" aria-live="assertive">
            <span className="toast-wave-icon"><Bell size={17} /></span>
            <span className="toast-content">
              <strong>High swell at {swellAlert.beach_name}</strong>
              <span>
                {swellAlert.wave_height.toFixed(1)} m waves ·{" "}
                {new Date(swellAlert.timestamp).toLocaleTimeString([], {
                  hour: "numeric",
                  minute: "2-digit",
                })}
              </span>
            </span>
            <button
              className="toast-dismiss"
              aria-label="Dismiss swell alert"
              onClick={() => setSwellAlert(null)}
            >
              <X size={16} />
            </button>
          </aside>
        )}

        <div className="map-wrap">
          <SurfMap
            beaches={beaches}
            selectedBeach={selectedBeach}
            focusBeach={mapFocus}
            onSelect={(beach) => {
              setSelectedBeach(beach);
              setMapFocus(beach);
            }}
          />

          <div className="map-label map-label-north">N <span>↑</span></div>
          <div className="map-coordinates">
            <span className="coordinate-crosshair">⌖</span>
            <span>32°05&apos;N&nbsp; 34°46&apos;E</span>
          </div>

          {selectedBeach && (
            <article className="selected-spot">
              <div className="spot-topline">
                <span className="spot-live"><i /> BEACH CONDITIONS</span>
                <span className="spot-rating">✦ <b>4.8</b></span>
              </div>
              <div className="spot-title-row">
                <div>
                  <div className="spot-region">
                    {selectedBeach.latitude > 32.5 ? "HAIFA DISTRICT" : "TEL AVIV DISTRICT"}
                  </div>
                  <h2>{selectedBeach.name}</h2>
                </div>
                <div className="spot-wave">
                  <strong>{selectedBeach.wave_height.toFixed(1)}<small>m</small></strong>
                  <span>SWELL</span>
                </div>
              </div>
              <p className="spot-description">
                {selectedBeach.description || "A local favorite on the Mediterranean coast."}
              </p>
              <div className="spot-divider" />
              <div className="spot-data-note">LOCAL SEA SNAPSHOT · SAMPLE DETAILS</div>
              <div className="spot-stats">
                <div className="spot-stat">
                  <span className="stat-icon wind-stat"><Wind size={16} /></span>
                  <span><small>WIND</small><b>{selectedBeach.wind_speed.toFixed(0)} <i>kn</i></b></span>
                </div>
                <div className="spot-stat">
                  <span className="stat-icon wave-stat"><Waves size={16} /></span>
                  <span><small>WAVE PERIOD</small><b>{selectedBeach.wave_period_seconds.toFixed(1)} <i>s</i></b></span>
                </div>
                <div className="spot-stat">
                  <span className="stat-icon water-stat"><Waves size={16} /></span>
                  <span><small>WATER TEMP</small><b>{selectedBeach.water_temperature_celsius.toFixed(1)} <i>°C</i></b></span>
                </div>
                <div className="spot-stat">
                  <span className="stat-icon tide-stat"><ArrowUpRight size={16} /></span>
                  <span><small>TIDE · {selectedBeach.tide_phase.toUpperCase()}</small><b>{selectedBeach.tide_height_meters.toFixed(2)} <i>m</i></b></span>
                </div>
              </div>
              <div className="spot-card-bottom">
                <span className="spot-coordinates">
                  <MapPin size={13} /> {selectedBeach.latitude.toFixed(4)}° N, {selectedBeach.longitude.toFixed(4)}° E
                </span>
                <span className="spot-direction">SWELL {compassDirection(selectedBeach.swell_direction_degrees)} · {selectedBeach.swell_direction_degrees}°</span>
                <span className={`spot-rating-text ${getSurfStatus(selectedBeach.wave_height).color}`}>
                  {getSurfStatus(selectedBeach.wave_height).label}
                </span>
              </div>
            </article>
          )}

          <div className="map-legend">
            <span className="legend-label">SURF INDEX</span>
            <span><i className="legend-dot good" /> GOOD</span>
            <span><i className="legend-dot fair" /> FAIR</span>
            <span><i className="legend-dot small" /> SMALL</span>
          </div>
        </div>
        <footer className="map-footer">
          <span>© OPENSTREETMAP CONTRIBUTORS</span>
          <span>OCEAN DATA <b>OPEN-METEO MARINE</b></span>
          <span className="map-footer-right">SALT IN THE AIR. <b>✳</b></span>
        </footer>
      </section>
      {authDialogOpen && (
        <AuthDialog
          onClose={() => setAuthDialogOpen(false)}
          onAuthenticated={handleAuthenticated}
        />
      )}
      {suggestionDialogOpen && authSession && (
        <BeachSuggestionDialog
          accessToken={authSession.accessToken}
          onClose={() => setSuggestionDialogOpen(false)}
          onCreated={handleBeachCreated}
        />
      )}
    </main>
  );
}
