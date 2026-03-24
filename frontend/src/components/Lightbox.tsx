import { useState, useEffect, useCallback } from 'react';
import type { Attachment } from '../types';

interface Props {
  attachment: Attachment;
  allImages: Attachment[];
  onClose: () => void;
}

export default function Lightbox({ attachment, allImages, onClose }: Props) {
  const [currentIndex, setCurrentIndex] = useState(() =>
    allImages.findIndex((a) => a.id === attachment.id)
  );
  const current = allImages[currentIndex] || attachment;
  const hasMultiple = allImages.length > 1;

  const goNext = useCallback(() => {
    setCurrentIndex((i) => (i < allImages.length - 1 ? i + 1 : 0));
  }, [allImages.length]);

  const goPrev = useCallback(() => {
    setCurrentIndex((i) => (i > 0 ? i - 1 : allImages.length - 1));
  }, [allImages.length]);

  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
      if (e.key === 'ArrowRight') goNext();
      if (e.key === 'ArrowLeft') goPrev();
    };
    window.addEventListener('keydown', handleKey);
    return () => window.removeEventListener('keydown', handleKey);
  }, [onClose, goNext, goPrev]);

  return (
    <div
      className="fixed inset-0 z-50 bg-black/90 flex items-center justify-center"
      onClick={onClose}
    >
      <button
        onClick={onClose}
        className="absolute top-4 right-4 text-white/70 hover:text-white p-2 transition-colors z-10"
      >
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" className="w-8 h-8">
          <path fillRule="evenodd" d="M5.47 5.47a.75.75 0 011.06 0L12 10.94l5.47-5.47a.75.75 0 111.06 1.06L13.06 12l5.47 5.47a.75.75 0 11-1.06 1.06L12 13.06l-5.47 5.47a.75.75 0 01-1.06-1.06L10.94 12 5.47 6.53a.75.75 0 010-1.06z" clipRule="evenodd" />
        </svg>
      </button>

      <a
        href={`/api/v1/files/${current.id}/download`}
        download
        onClick={(e) => e.stopPropagation()}
        className="absolute top-4 right-16 text-white/70 hover:text-white p-2 transition-colors z-10"
        title="Download"
      >
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" className="w-7 h-7">
          <path fillRule="evenodd" d="M12 2.25a.75.75 0 01.75.75v11.69l3.22-3.22a.75.75 0 111.06 1.06l-4.5 4.5a.75.75 0 01-1.06 0l-4.5-4.5a.75.75 0 111.06-1.06l3.22 3.22V3a.75.75 0 01.75-.75zm-9 13.5a.75.75 0 01.75.75v2.25a1.5 1.5 0 001.5 1.5h13.5a1.5 1.5 0 001.5-1.5V16.5a.75.75 0 011.5 0v2.25a3 3 0 01-3 3H5.25a3 3 0 01-3-3V16.5a.75.75 0 01.75-.75z" clipRule="evenodd" />
        </svg>
      </a>

      {hasMultiple && (
        <>
          <button
            onClick={(e) => { e.stopPropagation(); goPrev(); }}
            className="absolute left-4 top-1/2 -translate-y-1/2 text-white/70 hover:text-white p-2 transition-colors z-10"
          >
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" className="w-10 h-10">
              <path fillRule="evenodd" d="M7.72 12.53a.75.75 0 010-1.06l7.5-7.5a.75.75 0 111.06 1.06L9.31 12l6.97 6.97a.75.75 0 11-1.06 1.06l-7.5-7.5z" clipRule="evenodd" />
            </svg>
          </button>
          <button
            onClick={(e) => { e.stopPropagation(); goNext(); }}
            className="absolute right-4 top-1/2 -translate-y-1/2 text-white/70 hover:text-white p-2 transition-colors z-10"
          >
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" className="w-10 h-10">
              <path fillRule="evenodd" d="M16.28 11.47a.75.75 0 010 1.06l-7.5 7.5a.75.75 0 01-1.06-1.06L14.69 12 7.72 5.03a.75.75 0 011.06-1.06l7.5 7.5z" clipRule="evenodd" />
            </svg>
          </button>
        </>
      )}

      <img
        src={`/api/v1/files/${current.id}`}
        alt={current.original_name}
        className="max-w-[90vw] max-h-[85vh] object-contain rounded-lg"
        onClick={(e) => e.stopPropagation()}
      />

      <div className="absolute bottom-4 left-1/2 -translate-x-1/2 text-white/60 text-sm text-center">
        <div>{current.original_name}</div>
        {hasMultiple && (
          <div className="text-xs mt-1">{currentIndex + 1} / {allImages.length}</div>
        )}
      </div>
    </div>
  );
}
