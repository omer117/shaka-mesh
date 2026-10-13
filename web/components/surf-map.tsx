"use client";

import { useEffect } from "react";
import {
  CircleMarker,
  MapContainer,
  TileLayer,
  Tooltip,
  useMap,
} from "react-leaflet";
import L from "leaflet";
import type { Beach } from "@/lib/api";

const coastCenter: [number, number] = [32.37, 34.83];

interface SurfMapProps {
  beaches: Beach[];
  selectedBeach: Beach | null;
  focusBeach: Beach | null;
  onSelect: (beach: Beach) => void;
}

function MapFocus({ beach }: { beach: Beach | null }) {
  const map = useMap();

  useEffect(() => {
    if (beach) {
      map.flyTo([beach.latitude, beach.longitude], 12, {
        duration: 1.2,
      });
    }
  }, [beach, map]);

  return null;
}

function waveColor(height: number) {
  if (height >= 1.5) return "#ff795f";
  if (height >= 0.8) return "#53a789";
  return "#55a9c7";
}

function createBeachIcon(beach: Beach, selected: boolean) {
  const color = waveColor(beach.wave_height);
  const size = selected ? 46 : 38;
  return L.divIcon({
    className: "surf-marker-wrapper",
    html: `<div class="surf-marker ${selected ? "surf-marker-selected" : ""}" style="--marker-color:${color};--marker-size:${size}px"><span>🌊</span></div>`,
    iconSize: [size, size],
    iconAnchor: [size / 2, size / 2],
  });
}

export default function SurfMap({
  beaches,
  selectedBeach,
  focusBeach,
  onSelect,
}: SurfMapProps) {
  return (
    <MapContainer
      center={coastCenter}
      zoom={8}
      scrollWheelZoom
      zoomControl={false}
      className="surf-leaflet-map"
    >
      <TileLayer
        attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
        url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
      />
      <MapFocus beach={focusBeach} />
      {beaches.map((beach) => {
        const selected = selectedBeach?.id === beach.id;
        return (
          <CircleMarker
            key={beach.id}
            center={[beach.latitude, beach.longitude]}
            radius={selected ? 0 : 3}
            pathOptions={{
              color: waveColor(beach.wave_height),
              fillColor: waveColor(beach.wave_height),
              fillOpacity: 0.5,
              weight: 0,
            }}
            eventHandlers={{ click: () => onSelect(beach) }}
          >
            <Tooltip direction="top" offset={[0, -12]}>
              <span className="map-tooltip-name">{beach.name}</span>
              <span className="map-tooltip-height">{beach.wave_height.toFixed(1)} m swell</span>
            </Tooltip>
          </CircleMarker>
        );
      })}
      {beaches.map((beach) => (
        <SurfPin
          key={`pin-${beach.id}`}
          beach={beach}
          selected={selectedBeach?.id === beach.id}
          onSelect={onSelect}
        />
      ))}
    </MapContainer>
  );
}

function SurfPin({
  beach,
  selected,
  onSelect,
}: {
  beach: Beach;
  selected: boolean;
  onSelect: (beach: Beach) => void;
}) {
  const map = useMap();

  useEffect(() => {
    const marker = L.marker([beach.latitude, beach.longitude], {
      icon: createBeachIcon(beach, selected),
      keyboard: true,
      title: beach.name,
      alt: `${beach.name}, ${beach.wave_height.toFixed(1)} meter swell`,
    });
    marker.addTo(map);
    marker.on("click", () => onSelect(beach));
    return () => {
      marker.off();
      marker.remove();
    };
  }, [beach, map, onSelect, selected]);

  return null;
}
