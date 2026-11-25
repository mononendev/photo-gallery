const link = 'text-blue-400 hover:underline';

export default function About() {
  return (
    <article className="mx-auto max-w-3xl space-y-4 leading-relaxed text-gray-300">
      <h1 className="text-3xl font-semibold text-gray-100">About Me</h1>
      <p className="text-lg text-gray-400">OneWheeler and Photographer</p>
      <img alt="Alex" width={1016} height={1267} loading="lazy"
        src="https://lh3.googleusercontent.com/pw/AP1GczPlrz8nsQJcMK2prZywMYiwAG6U31kirsGPay8aGOzlydjzyh3RXnXv8cLhEQ-pYcpr3UL64y-dkqeh6_jQMPovFFMYVqaRhXB79EeGFvm4Ugtq-9I27ZNKND8g2uONR_XsVynwE8PFw4VQ51Ctp2ZeEQ=w1016-h1267-s-no?authuser=0"
        className="h-auto w-full max-w-sm rounded-lg" />
      <p>
        Howdy, welcome to my photography portfolio. My name is Alex, and I'm typically riding around a OneWheel event with a big camera.
      </p>
      <h2 className="pt-2 text-2xl font-semibold text-gray-100">My Story</h2>
      <p>
        I started photography as a hobby when I was in high school and took a few classes on film photography in college, and fell in love with capturing things that happened in my life.
      </p>
      <p>
        In 2021 I tried out a friend's OneWheel and was hooked. I bought my own Pint, XR, then began building my own.
        Moved down to the Triangle of NC, and joined{' '}
        <a className={link} href="https://www.oakcityonewheel.com/" target="_blank" rel="noreferrer">Oak City OneWheel</a>,
        where I met some lifelong friends.
      </p>
      <h2 className="pt-2 text-2xl font-semibold text-gray-100">Hardware</h2>
      <h3 className="text-xl font-semibold text-gray-100">Cameras</h3>
      <p>
        I have a Sony A7RII that I will shoot on, or more often 'loan' out to friends.
        I also frequently borrow a camera or two from{' '}
        <a className={link} href="https://www.facebook.com/cory.boehne" target="_blank" rel="noreferrer">Cory Boehne</a> of{' '}
        <a className={link} href="https://armor-dilloz.com/" target="_blank" rel="noreferrer">Armor-Dilloz</a>
      </p>
      <h3 className="text-xl font-semibold text-gray-100">Boards</h3>
      <p>
        I'm most likely riding a board that I have built, certainly a vesc of some variety.
        Latest hardware is some fun 30s odds and ends built by myself at{' '}
        <a className={link} href="https://monomotion.org" target="_blank" rel="noreferrer">MonoMotion</a>
      </p>
    </article>
  );
}
