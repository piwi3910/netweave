"use client";

import { useEffect, useState } from "react";
import { Link2 } from "lucide-react";
import { CardSkeleton } from "@/components/shared/loading-skeleton";
import { EmptyState } from "@/components/shared/empty-state";
import {
  listBackends,
  listDMSLinks,
  createDMSLink,
  deleteDMSLink,
} from "@/lib/api/backends";
import { getDisplayError } from "@/lib/utils/sanitize-error";
import type { BackendResponse } from "@/types/backends";

export default function LinksPage() {
  const [imsBackends, setImsBackends] = useState<BackendResponse[]>([]);
  const [dmsBackends, setDmsBackends] = useState<BackendResponse[]>([]);
  const [links, setLinks] = useState<Record<string, Set<string>>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [toggling, setToggling] = useState<string | null>(null);

  useEffect(() => {
    loadData();
  }, []);

  async function loadData() {
    try {
      setLoading(true);
      const [imsData, dmsData] = await Promise.all([
        listBackends("ims"),
        listBackends("dms"),
      ]);
      setImsBackends(imsData);
      setDmsBackends(dmsData);

      const linksMap: Record<string, Set<string>> = {};
      await Promise.all(
        imsData.map(async (ims) => {
          const dmsLinks = await listDMSLinks(ims.id);
          linksMap[ims.id] = new Set(dmsLinks.map((d) => d.id));
        })
      );
      setLinks(linksMap);
    } catch (err) {
      setError(getDisplayError(err));
    } finally {
      setLoading(false);
    }
  }

  async function toggleLink(imsId: string, dmsId: string, isLinked: boolean) {
    const key = `${imsId}-${dmsId}`;
    setToggling(key);
    try {
      if (isLinked) {
        await deleteDMSLink(imsId, dmsId);
        setLinks((prev) => {
          const newLinks = { ...prev };
          newLinks[imsId] = new Set(newLinks[imsId]);
          newLinks[imsId].delete(dmsId);
          return newLinks;
        });
      } else {
        await createDMSLink(imsId, dmsId);
        setLinks((prev) => {
          const newLinks = { ...prev };
          if (!newLinks[imsId]) {
            newLinks[imsId] = new Set();
          } else {
            newLinks[imsId] = new Set(newLinks[imsId]);
          }
          newLinks[imsId].add(dmsId);
          return newLinks;
        });
      }
    } catch (err) {
      setError(getDisplayError(err));
    } finally {
      setToggling(null);
    }
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <h1 className="text-3xl font-bold">IMS-DMS Links</h1>
        <CardSkeleton />
      </div>
    );
  }

  if (imsBackends.length === 0 || dmsBackends.length === 0) {
    return (
      <div className="space-y-6">
        <h1 className="text-3xl font-bold">IMS-DMS Links</h1>
        <EmptyState
          icon={Link2}
          title="No backends available"
          description="Create at least one O-Cloud and one DMS to manage links"
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <h1 className="text-3xl font-bold">IMS-DMS Links</h1>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}

      <div className="rounded-lg border bg-card overflow-x-auto">
        <table className="w-full">
          <thead>
            <tr className="border-b bg-muted/50">
              <th className="p-4 text-left font-medium sticky left-0 bg-muted/50 z-10">
                O-Cloud / DMS
              </th>
              {dmsBackends.map((dms) => (
                <th
                  key={dms.id}
                  className="p-4 text-center font-medium min-w-32"
                >
                  <div className="text-sm">{dms.name}</div>
                  <div className="text-xs text-muted-foreground font-normal">
                    {dms.adapterType}
                  </div>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {imsBackends.map((ims) => (
              <tr key={ims.id} className="border-b">
                <td className="p-4 font-medium sticky left-0 bg-card border-r">
                  <div className="text-sm">{ims.name}</div>
                  <div className="text-xs text-muted-foreground">
                    {ims.adapterType}
                  </div>
                </td>
                {dmsBackends.map((dms) => {
                  const isLinked = links[ims.id]?.has(dms.id) || false;
                  const key = `${ims.id}-${dms.id}`;
                  const isToggling = toggling === key;
                  return (
                    <td key={dms.id} className="p-4 text-center">
                      <input
                        type="checkbox"
                        checked={isLinked}
                        disabled={isToggling}
                        onChange={() => toggleLink(ims.id, dms.id, isLinked)}
                        className="h-5 w-5 rounded border-gray-300 cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
                      />
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="rounded-lg border bg-muted/50 p-4">
        <p className="text-sm text-muted-foreground">
          Check a box to link an O-Cloud (IMS) with a DMS. Uncheck to remove
          the link. Links enable the O-Cloud to deploy packages from the DMS.
        </p>
      </div>
    </div>
  );
}
