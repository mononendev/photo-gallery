import { useState } from 'react';

const KEY = 'disclaimerAcceptedAt';
const FIFTEEN_MINUTES = 15 * 60 * 1000;

function needsAccept(): boolean {
  const at = localStorage.getItem(KEY);
  if (!at) return true;
  return Date.now() - parseInt(at, 10) > FIFTEEN_MINUTES;
}

export default function DisclaimerModal() {
  const [open, setOpen] = useState(needsAccept);
  if (!open) return null;

  const accept = () => {
    localStorage.setItem(KEY, Date.now().toString());
    setOpen(false);
  };

  return (
    <div role="dialog" aria-modal="true" aria-labelledby="disclaimer-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm sm:p-4">
      <div className="w-full h-full sm:h-auto sm:max-w-xl sm:max-h-[90vh] overflow-y-auto sm:rounded-2xl bg-gray-900 border border-gray-800 p-6 sm:p-8 shadow-2xl">
        <h2 id="disclaimer-title" className="text-2xl sm:text-3xl font-bold tracking-tight mb-4">Photo Usage Terms</h2>
        <p className="mb-4 leading-relaxed">
          Before viewing and accessing the photos in this gallery, please read and accept the following terms:
        </p>
        <ul className="list-disc pl-5 mb-4 space-y-3 leading-relaxed">
          <li>
            <strong>Personal Use Only:</strong> These photos are provided solely for your personal viewing and enjoyment. Any use, reproduction, distribution, or exploitation beyond personal use is strictly prohibited without the prior written consent of the photographer.
          </li>
          <li>
            <strong>No AI Training:</strong> You may not use these photos to train, fine-tune, or develop any artificial intelligence models, machine learning systems, or automated image recognition systems.
          </li>
          <li>
            <strong>No Public AI Model Input:</strong> Do not upload or submit these photos to publicly available AI tools or services (such as image generators, chatbots, or search engines). Many such services use submitted content to train or improve their models.
          </li>
          <li>
            <strong>Attribution Required:</strong> These photos are provided free of charge. If you share or post these photos elsewhere, please give credit to the photographer by tagging or mention.
            <span className="block mt-1 text-sm text-gray-400">
              Using a photo as your profile picture? Cropping may cut off the watermark—please credit the photographer in a comment or your post instead!
            </span>
          </li>
        </ul>
        <p className="mb-6 italic text-gray-400 leading-relaxed">
          By clicking &ldquo;I Accept the Terms&rdquo; below, you acknowledge that you have read and agree to abide by these terms.
        </p>
        <button onClick={accept} className="w-full rounded-lg bg-blue-600 hover:bg-blue-500 py-3 text-lg font-semibold">
          I Accept the Terms
        </button>
      </div>
    </div>
  );
}
