// OPENFRAME(winget-packages): a winget_packages row always has a programs twin for the same install, which carries the
// CVEs with the right target platform; a CPE for the winget row would duplicate them and, with no target_sw, also match
// CVEs written for other platforms
//
// OPENFRAME(homebrew-casks): the same for a homebrew_casks row and its apps twin, which has the bundle identifier the
// CVE matching needs
package nvd

var openframeNoCPESources = []string{"winget_packages", "homebrew_casks"}
