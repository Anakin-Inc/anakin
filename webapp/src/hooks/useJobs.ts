import { useState, useCallback, useEffect } from 'react';
import type { TrackedJob } from '../types';

const STORAGE_KEY = 'anakinscraper_jobs';
const MAX_JOBS = 100;

function loadJobs(): TrackedJob[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
}

function saveJobs(jobs: TrackedJob[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(jobs.slice(0, MAX_JOBS)));
}

export function useJobs() {
  const [jobs, setJobs] = useState<TrackedJob[]>(loadJobs);

  useEffect(() => {
    saveJobs(jobs);
  }, [jobs]);

  const addJob = useCallback((job: TrackedJob) => {
    setJobs((prev) => [job, ...prev]);
  }, []);

  const updateJob = useCallback((id: string, updates: Partial<TrackedJob>) => {
    setJobs((prev) =>
      prev.map((j) => (j.id === id ? { ...j, ...updates } : j))
    );
  }, []);

  const removeJob = useCallback((id: string) => {
    setJobs((prev) => prev.filter((j) => j.id !== id));
  }, []);

  const clearJobs = useCallback(() => {
    setJobs([]);
  }, []);

  return { jobs, addJob, updateJob, removeJob, clearJobs };
}
