/**
 * is-cloud-customer
 *
 * A simple policy that allows requests to microsoft proxy endpoints from a cloud customer.
 *
 * For more about how to use policies, see:
 *   https://sailsjs.com/config/policies
 *   https://sailsjs.com/docs/concepts/policies
 *   https://sailsjs.com/docs/concepts/policies/access-control-and-permissions
 */
const crypto = require('crypto');

module.exports = async function (req, res, proceed) {

  // If an MS API KEY header was provided, check to see if it matches the entraSharedSecret.
  if (req.get('MS-API-KEY')) {
    let providedKey = Buffer.from(req.get('MS-API-KEY'));
    let matchesSecret = [sails.config.custom.cloudCustomerCompliancePartnerSharedSecret, sails.config.custom.alternateCompliancePartnerSharedSecret].some((configuredSecret) => {
      if (!configuredSecret) { return false; }
      let configuredKey = Buffer.from(configuredSecret);
      if (configuredKey.length !== providedKey.length) { return false; }
      return crypto.timingSafeEqual(providedKey, configuredKey);
    });
    if (matchesSecret) {
      return proceed();
    }
  }

  //--•
  // Otherwise, this request did not come from a cloud customer.
  return res.unauthorized();

};
