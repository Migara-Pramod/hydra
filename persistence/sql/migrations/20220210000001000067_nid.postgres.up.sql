-- Migration generated by the command below; DO NOT EDIT.
-- hydra:generate hydra migrate gen

CREATE INDEX hydra_oauth2_trusted_jwt_bearer_issuer_expires_at_idx ON hydra_oauth2_trusted_jwt_bearer_issuer (expires_at ASC);
CREATE INDEX hydra_oauth2_trusted_jwt_bearer_issuer_nid_idx ON hydra_oauth2_trusted_jwt_bearer_issuer (id, nid);
