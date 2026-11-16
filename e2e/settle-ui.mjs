// Axe measures the rendered state after fonts and finite entrance transitions settle.
// Waiting for animation completion keeps the audit independent of runner speed.
export async function settleUI(page) {
  await page.evaluate(async () => {
    await document.fonts.ready;
    await Promise.all(document.getAnimations().filter(animation => animation.playState === 'running' && animation.effect?.getTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {})));
  });
}
